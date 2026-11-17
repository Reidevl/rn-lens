package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// frameRecord is one line of a recording. Recordings feed the fake proxy in
// tests, so they capture both directions with timing.
type frameRecord struct {
	At    time.Duration   `json:"at"`
	Dir   cdp.Direction   `json:"dir"`
	Frame json.RawMessage `json:"frame"`
}

// runRecord attaches to a target and writes every CDP frame to a JSONL file
// until the duration elapses or Ctrl-C.
func runRecord(args []string) int {
	fs := flag.NewFlagSet("rnlens record", flag.ContinueOnError)
	endpoints := fs.String("metro", strings.Join(metro.DefaultEndpoints, ","), "comma-separated Metro base URLs to poll")
	targetID := fs.String("target", "", "target id from `rnlens targets` (default: first reloadable target)")
	out := fs.String("out", "", "output file (JSONL); required")
	duration := fs.Duration("duration", 30*time.Second, "how long to record (0 = until Ctrl-C)")
	enableRuntime := fs.Bool("runtime", true, "also enable the Runtime domain so console traffic is recorded")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "rnlens record: -out is required")
		return 2
	}

	// The output file is created only once a target exists (below), so a
	// failed run leaves no empty recording behind. The observer only fires
	// after Attach, by which time writer is set.
	start := time.Now()
	var mu sync.Mutex
	var writer *bufio.Writer
	frames := 0
	observer := func(dir cdp.Direction, raw []byte) {
		mu.Lock()
		defer mu.Unlock()
		line, _ := json.Marshal(frameRecord{At: time.Since(start), Dir: dir, Frame: json.RawMessage(raw)})
		_, _ = writer.Write(line)
		_ = writer.WriteByte('\n')
		frames++
	}

	core := app.New(app.Config{Version: version, MetroEndpoints: splitList(*endpoints), FrameObserver: observer})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	core.RefreshTargets(ctx)
	cancel()

	id := *targetID
	if id == "" {
		for _, t := range core.Targets() {
			if t.Reloadable {
				id = t.ID
				break
			}
		}
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "rnlens record: no target found; start your app and Metro first")
		return 2
	}

	file, err := os.Create(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rnlens record: %v\n", err)
		return 1
	}
	defer file.Close()
	mu.Lock()
	writer = bufio.NewWriter(file)
	mu.Unlock()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		_ = writer.Flush()
	}()
	sessionCh, cancelSession := core.Bus().Subscribe(app.TopicSessionChanged, 64)
	defer cancelSession()
	go func() {
		for range sessionCh {
			snap := core.SessionSnapshot()
			fmt.Fprintf(os.Stderr, "session: %s %s\n", snap.State, snap.Reason)
		}
	}()

	if err := core.Attach(id); err != nil {
		fmt.Fprintf(os.Stderr, "rnlens record: %v\n", err)
		return 1
	}
	defer core.Detach()
	fmt.Fprintf(os.Stderr, "recording %s to %s\n", id, *out)

	if *enableRuntime {
		go func() {
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if core.SessionSnapshot().State == targets.StateAttached {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = core.Session().Call(ctx, cdp.MethodRuntimeEnable, nil)
					cancel()
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	var timer <-chan time.Time
	if *duration > 0 {
		timer = time.After(*duration)
	}
	select {
	case <-stop:
	case <-timer:
	}

	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(os.Stderr, "recorded %d frames in %s\n", frames, time.Since(start).Round(time.Millisecond))
	return 0
}
