package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/fastrefresh"
	"github.com/Reidevl/rn-lens/internal/perf"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// TestE2E_PerformanceRecordings attaches to a real app (LENS_E2E_METRO, as in
// TestE2E_AttachToRunningApp) and takes a screenshot, a trace, a CPU profile
// and a heap snapshot, then checks that each one parses. It also waits for
// the Fast Refresh registration. The app must be the only React Native host
// (tracing refuses otherwise) and must not have a JavaScript debugger with
// the Debugger domain enabled. LENS_E2E_TRACE_OUT, when set, is where the
// trace is written, to open it in Perfetto UI. LENS_E2E_FAST_REFRESH_FILE,
// when set to a source file of the app, gets a comment appended (then
// removed) and the test waits for the Fast Refresh it triggers.
func TestE2E_PerformanceRecordings(t *testing.T) {
	endpoint := os.Getenv("LENS_E2E_METRO")
	if endpoint == "" || testing.Short() {
		t.Skip("set LENS_E2E_METRO=http://localhost:8081 with an app running to enable")
	}
	a := app.New(app.Config{MetroEndpoints: []string{endpoint}, PollInterval: 200 * time.Millisecond, SDKAddr: "off"})
	a.Start()
	t.Cleanup(a.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	a.RefreshTargets(ctx)
	var target targets.Target
	for _, candidate := range a.Targets() {
		if candidate.Reloadable {
			target = candidate
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("no reloadable target at %s", endpoint)
	}
	if err := a.Attach(target.ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for !ok() {
			select {
			case <-ctx.Done():
				t.Fatalf("timed out waiting for %s (Fast Refresh %+v)", what, a.FastRefresh())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	waitFor("the attach", func() bool { return a.SessionSnapshot().State == targets.StateAttached })
	waitFor("Fast Refresh", func() bool {
		p := a.FastRefresh().Phase
		return p == fastrefresh.PhaseReady || p == fastrefresh.PhaseUnavailable || p == fastrefresh.PhaseUpdated
	})
	if st := a.FastRefresh(); st.Phase == fastrefresh.PhaseUnavailable && strings.HasPrefix(st.Message, "Reload the app") {
		// Metro restarted since the app loaded its bundle: reload it, as the
		// hint says, and Lens registers again on its own.
		t.Logf("fast refresh: %s; reloading", st.Message)
		if err := a.Reload(ctx); err != nil {
			t.Fatalf("Reload: %v", err)
		}
		waitFor("Fast Refresh after the reload", func() bool { return a.FastRefresh().Phase == fastrefresh.PhaseReady })
		waitFor("the re-attach", func() bool { return a.SessionSnapshot().State == targets.StateAttached })
	}
	if st := a.FastRefresh(); st.Phase == fastrefresh.PhaseUnavailable {
		t.Errorf("Fast Refresh unavailable: %s", st.Message)
	}
	if file := os.Getenv("LENS_E2E_FAST_REFRESH_FILE"); file != "" {
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(file, original, 0o600) })
		edited := append(append([]byte(nil), original...), []byte("\n// lens-e2e fast refresh\n")...)
		if err := os.WriteFile(file, edited, 0o600); err != nil {
			t.Fatal(err)
		}
		waitFor("a Fast Refresh", func() bool { return a.FastRefresh().Phase == fastrefresh.PhaseUpdated })
		t.Logf("fast refresh: %+v", a.FastRefresh())
	}
	p := a.Performance()

	img, err := p.Screenshot(ctx, "png", 0)
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if !bytes.HasPrefix(img.Data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("screenshot is not a PNG (%d bytes)", len(img.Data))
	}
	t.Logf("screenshot: %d bytes", len(img.Data))

	work := "(() => { performance.mark('lens-e2e'); let x = 0; for (let i = 0; i < 2e6; i++) x += Math.sqrt(i); performance.measure('lens-e2e-work', 'lens-e2e'); return x > 0; })()"
	if err := p.StartTrace(ctx, nil); err != nil {
		t.Fatalf("StartTrace: %v", err)
	}
	if _, err := a.Console().Evaluate(ctx, work); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	time.Sleep(time.Second)
	trace, err := p.StopTrace(ctx)
	if err != nil {
		t.Fatalf("StopTrace: %v", err)
	}
	var file bytes.Buffer
	if err := perf.WriteTrace(&file, trace); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		TraceEvents []json.RawMessage `json:"traceEvents"`
	}
	if err := json.Unmarshal(file.Bytes(), &parsed); err != nil || len(parsed.TraceEvents) == 0 {
		t.Fatalf("trace file: %d events (%v)", len(parsed.TraceEvents), err)
	}
	t.Logf("trace: %+v", trace.Summarize())
	if out := os.Getenv("LENS_E2E_TRACE_OUT"); out != "" {
		if err := os.WriteFile(out, file.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := p.StartProfile(ctx); err != nil {
		t.Fatalf("StartProfile: %v", err)
	}
	if _, err := a.Console().Evaluate(ctx, work); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	profile, err := p.StopProfile(ctx)
	if err != nil {
		t.Fatalf("StopProfile: %v", err)
	}
	if s := perf.SummarizeProfile(profile); s.Nodes == 0 {
		t.Fatalf("profile has no nodes: %s", truncate(string(profile), 300))
	} else {
		t.Logf("profile: %+v", s)
	}

	var heap bytes.Buffer
	n, err := p.HeapSnapshot(ctx, &heap)
	if err != nil {
		t.Fatalf("HeapSnapshot: %v", err)
	}
	var snapshot struct {
		Snapshot json.RawMessage `json:"snapshot"`
		Nodes    []int64         `json:"nodes"`
	}
	if err := json.Unmarshal(heap.Bytes(), &snapshot); err != nil || len(snapshot.Snapshot) == 0 || len(snapshot.Nodes) == 0 {
		t.Fatalf("heap snapshot of %d bytes does not parse: %v", n, err)
	}
	t.Logf("heap snapshot: %d bytes, %d node fields", n, len(snapshot.Nodes))
}
