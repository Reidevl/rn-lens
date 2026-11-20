// Package perf records performance data from the attached app over CDP:
// screenshots, Chrome traces, CPU profiles and Hermes heap snapshots.
package perf

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Caller sends CDP commands to the attached app. Barrier queues a
// cdp.EventLensBarrier carrying token behind every event already received,
// so HandleEvent sees it after all of them.
type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Barrier(token string) error
}

// methodNotFound is the JSON-RPC code for a method the target lacks.
const methodNotFound = -32601

// Errors returned by the recording methods.
var (
	// ErrBusy means the same kind of recording is already running.
	ErrBusy = errors.New("perf: a recording of this kind is already running")
	// ErrNotRecording means Stop was called without a running recording.
	ErrNotRecording = errors.New("perf: nothing is being recorded")
	// ErrTraceIncomplete means the app never reported the end of the trace.
	ErrTraceIncomplete = errors.New("perf: the app did not finish sending the trace")
	// ErrSessionEnded means the debugger session changed during the operation.
	ErrSessionEnded = errors.New("perf: the app was detached or reloaded")
	// ErrScreenshotUnsupported means the app's debugger does not implement
	// Page.captureScreenshot (JSON-RPC "method not found").
	ErrScreenshotUnsupported = errors.New("this app does not support screenshots: its React Native debugger has no Page.captureScreenshot")
)

// DefaultTraceCategories are the categories React Native DevTools records.
var DefaultTraceCategories = []string{
	"devtools.timeline",
	"disabled-by-default-devtools.timeline",
	"disabled-by-default-devtools.timeline.frame",
	"blink.user_timing",
	"v8.execute",
	"disabled-by-default-v8.cpu_profiler",
}

// Options configures a Service.
type Options struct {
	// TraceTimeout bounds the wait for Tracing.tracingComplete after
	// Tracing.end. Zero means 30 seconds.
	TraceTimeout time.Duration
	// OnChange is called after Status changes. It must not block.
	OnChange func()
}

// Status reports the recordings in progress.
type Status struct {
	Tracing      bool `json:"tracing"`
	Profiling    bool `json:"profiling"`
	Snapshotting bool `json:"snapshotting"`
}

// Image is a captured screenshot.
type Image struct {
	Format string
	Data   []byte
}

// Trace is a finished recording: Chrome trace events in arrival order.
type Trace struct {
	Events   []json.RawMessage
	DataLoss bool
}

// Service runs one recording of each kind at a time. Feed it every event of
// the session with HandleEvent and call BeginSession on each attach.
type Service struct {
	caller  Caller
	timeout time.Duration
	notify  func()

	mu        sync.Mutex
	gen       uint64 // bumped by BeginSession; stale operations notice it
	ended     chan struct{}
	trace     *traceRec
	profiling bool
	stopping  bool // a Profiler.stop is in flight
	heap      *heapRec
	barriers  map[string]chan struct{}
	nextToken uint64
}

type traceRec struct {
	stopping bool
	events   []json.RawMessage
	dataLoss bool
	done     chan struct{}
	complete bool
}

// heapRec has its own lock so chunks are written without holding the
// service lock: Status and the other recordings never wait for the disk.
type heapRec struct {
	mu   sync.Mutex
	w    io.Writer
	n    int64
	err  error
	done bool // HeapSnapshot returned; w may be closed
}

func (h *heapRec) write(chunk string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.done || h.err != nil {
		return
	}
	n, err := io.WriteString(h.w, chunk)
	h.n += int64(n)
	h.err = err
}

func (h *heapRec) finish() (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.done = true
	return h.n, h.err
}

// NewService returns a Service that sends commands through caller.
func NewService(caller Caller, opts Options) *Service {
	timeout := opts.TraceTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	notify := opts.OnChange
	if notify == nil {
		notify = func() {}
	}
	return &Service{
		caller:   caller,
		timeout:  timeout,
		notify:   notify,
		ended:    make(chan struct{}),
		barriers: map[string]chan struct{}{},
	}
}

// Status returns the recordings in progress.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Tracing: s.trace != nil, Profiling: s.profiling, Snapshotting: s.heap != nil}
}

// BeginSession forgets every recording: a new debugger session cannot stop
// what the previous one started. Waiting operations return ErrSessionEnded.
// Call it on attach and on detach.
func (s *Service) BeginSession() {
	s.mu.Lock()
	s.gen++
	close(s.ended)
	s.ended = make(chan struct{})
	s.trace, s.profiling, s.stopping, s.heap = nil, false, false, nil
	s.barriers = map[string]chan struct{}{}
	s.mu.Unlock()
	s.notify()
}

// HandleEvent consumes the trace, heap snapshot and barrier events.
func (s *Service) HandleEvent(ev cdp.Event) {
	switch ev.Method {
	case "Tracing.dataCollected":
		var p struct {
			Value []json.RawMessage `json:"value"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			return
		}
		s.mu.Lock()
		// React Native sends chunks only after Tracing.end: chunks during a
		// recording are late ones from a stop that timed out.
		if s.trace != nil && s.trace.stopping {
			s.trace.events = append(s.trace.events, p.Value...)
		}
		s.mu.Unlock()
	case "Tracing.tracingComplete":
		var p struct {
			DataLossOccurred bool `json:"dataLossOccurred"`
		}
		_ = json.Unmarshal(ev.Params, &p)
		s.mu.Lock()
		if t := s.trace; t != nil && t.stopping && !t.complete {
			t.dataLoss, t.complete = p.DataLossOccurred, true
			close(t.done)
		}
		s.mu.Unlock()
	case "HeapProfiler.addHeapSnapshotChunk":
		var p struct {
			Chunk string `json:"chunk"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			return
		}
		s.mu.Lock()
		h := s.heap
		s.mu.Unlock()
		if h != nil {
			h.write(p.Chunk)
		}
	case cdp.EventLensBarrier:
		var token string
		if json.Unmarshal(ev.Params, &token) != nil {
			return
		}
		s.mu.Lock()
		if ch, ok := s.barriers[token]; ok {
			delete(s.barriers, token)
			close(ch)
		}
		s.mu.Unlock()
	}
}

// Screenshot captures the app's screen. format is "png" (default) or
// "jpeg"; quality (1-100) applies to JPEG only.
func (s *Service) Screenshot(ctx context.Context, format string, quality int) (Image, error) {
	if format == "" {
		format = "png"
	}
	params := map[string]any{"format": format}
	if format == "jpeg" && quality > 0 {
		params["quality"] = quality
	}
	raw, err := s.caller.Call(ctx, "Page.captureScreenshot", params)
	var protoErr *cdp.Error
	if errors.As(err, &protoErr) && protoErr.Code == methodNotFound {
		return Image{}, ErrScreenshotUnsupported
	}
	if err != nil {
		return Image{}, fmt.Errorf("perf: screenshot: %w", err)
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return Image{}, fmt.Errorf("perf: screenshot: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return Image{}, fmt.Errorf("perf: screenshot: %w", err)
	}
	return Image{Format: format, Data: data}, nil
}

// StartTrace starts a Chrome trace. Nil categories use
// DefaultTraceCategories.
func (s *Service) StartTrace(ctx context.Context, categories []string) error {
	if len(categories) == 0 {
		categories = DefaultTraceCategories
	}
	s.mu.Lock()
	if s.trace != nil {
		s.mu.Unlock()
		return ErrBusy
	}
	rec := &traceRec{done: make(chan struct{})}
	s.trace = rec
	s.mu.Unlock()
	s.notify()

	if _, err := s.caller.Call(ctx, "Tracing.start", map[string]any{"categories": joinCategories(categories)}); err != nil {
		s.dropTrace(rec)
		return fmt.Errorf("perf: start trace: %w", err)
	}
	return nil
}

// StopTrace ends the trace and waits for every chunk the app sends.
func (s *Service) StopTrace(ctx context.Context) (Trace, error) {
	s.mu.Lock()
	rec := s.trace
	if rec == nil {
		s.mu.Unlock()
		return Trace{}, ErrNotRecording
	}
	if rec.stopping {
		s.mu.Unlock()
		return Trace{}, ErrBusy
	}
	rec.stopping = true
	ended := s.ended
	s.mu.Unlock()
	defer s.dropTrace(rec)

	if _, err := s.caller.Call(ctx, "Tracing.end", nil); err != nil {
		return Trace{}, fmt.Errorf("perf: stop trace: %w", err)
	}
	timer := time.NewTimer(s.timeout)
	defer timer.Stop()
	select {
	case <-rec.done:
	case <-ended:
		return Trace{}, ErrSessionEnded
	case <-timer.C:
		return Trace{}, ErrTraceIncomplete
	case <-ctx.Done():
		return Trace{}, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Trace{Events: rec.events, DataLoss: rec.dataLoss}, nil
}

func (s *Service) dropTrace(rec *traceRec) {
	s.mu.Lock()
	changed := s.trace == rec
	if changed {
		s.trace = nil
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

// StartProfile starts the JavaScript sampling profiler.
func (s *Service) StartProfile(ctx context.Context) error {
	s.mu.Lock()
	if s.profiling {
		s.mu.Unlock()
		return ErrBusy
	}
	s.profiling = true
	gen := s.gen
	s.mu.Unlock()
	s.notify()
	if _, err := s.caller.Call(ctx, "Profiler.start", nil); err != nil {
		s.endProfile(gen)
		return fmt.Errorf("perf: start profile: %w", err)
	}
	return nil
}

// StopProfile stops the profiler and returns the Profiler.Profile JSON, the
// content of a .cpuprofile file.
func (s *Service) StopProfile(ctx context.Context) (json.RawMessage, error) {
	s.mu.Lock()
	if !s.profiling {
		s.mu.Unlock()
		return nil, ErrNotRecording
	}
	if s.stopping {
		s.mu.Unlock()
		return nil, ErrBusy
	}
	s.stopping = true
	gen := s.gen
	s.mu.Unlock()
	defer s.endProfile(gen)
	raw, err := s.caller.Call(ctx, "Profiler.stop", nil)
	if err != nil {
		return nil, fmt.Errorf("perf: stop profile: %w", err)
	}
	var res struct {
		Profile json.RawMessage `json:"profile"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.Profile) == 0 {
		return nil, fmt.Errorf("perf: stop profile: the app returned no profile")
	}
	return res.Profile, nil
}

func (s *Service) endProfile(gen uint64) {
	s.mu.Lock()
	changed := s.gen == gen && s.profiling
	if changed {
		s.profiling, s.stopping = false, false
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

// HeapSnapshot writes a Hermes heap snapshot (.heapsnapshot JSON) to w as
// the chunks arrive and returns the number of bytes written.
func (s *Service) HeapSnapshot(ctx context.Context, w io.Writer) (int64, error) {
	s.mu.Lock()
	if s.heap != nil {
		s.mu.Unlock()
		return 0, ErrBusy
	}
	rec := &heapRec{w: w}
	s.heap = rec
	s.nextToken++
	token := "heap-" + strconv.FormatUint(s.nextToken, 10)
	arrived := make(chan struct{})
	s.barriers[token] = arrived
	ended := s.ended
	s.mu.Unlock()
	s.notify()
	defer func() {
		_, _ = rec.finish() // no write may follow the return
		s.mu.Lock()
		changed := s.heap == rec
		if changed {
			s.heap = nil
		}
		delete(s.barriers, token)
		s.mu.Unlock()
		if changed {
			s.notify()
		}
	}()

	if _, err := s.caller.Call(ctx, "HeapProfiler.takeHeapSnapshot", map[string]any{"reportProgress": false}); err != nil {
		return 0, fmt.Errorf("perf: heap snapshot: %w", err)
	}
	// Chunks sent before the reply can still be queued behind it.
	if err := s.caller.Barrier(token); err != nil {
		return 0, fmt.Errorf("perf: heap snapshot: %w", err)
	}
	select {
	case <-arrived:
	case <-ended:
		return 0, ErrSessionEnded
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	n, err := rec.finish()
	if err != nil {
		return n, fmt.Errorf("perf: heap snapshot: %w", err)
	}
	return n, nil
}

func joinCategories(c []string) string { return strings.Join(c, ",") }
