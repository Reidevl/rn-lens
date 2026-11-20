package perf_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/perf"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// conn adapts a cdpws connection to perf.Caller and feeds its events to the
// service, like the targets session does.
type conn struct{ *cdpws.Conn }

func (c conn) Barrier(token string) error {
	if !c.Conn.Barrier(token) {
		return errors.New("closed")
	}
	return nil
}

func newService(t *testing.T, opts perf.Options) (*perf.Service, *cdptest.Server) {
	t.Helper()
	svc, srv, _ := newServiceWithFlush(t, opts)
	return svc, srv
}

// newServiceWithFlush also returns flush, which returns once the service
// has handled every event the server sent before the call.
func newServiceWithFlush(t *testing.T, opts perf.Options) (*perf.Service, *cdptest.Server, func()) {
	t.Helper()
	srv := cdptest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := cdpws.Dial(ctx, srv.URL(), cdpws.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	svc := perf.NewService(conn{c}, opts)
	flushed := make(chan struct{}, 1)
	go func() {
		for ev := range c.Events() {
			svc.HandleEvent(ev)
			if ev.Method == cdp.EventLensBarrier && string(ev.Params) == `"test-flush"` {
				flushed <- struct{}{}
			}
		}
	}()
	flush := func() {
		t.Helper()
		// The reply (an unknown-method error) follows every event sent
		// before it, so the barrier lands behind them all.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = c.Call(ctx, "Lens.testFlush", nil)
		if !c.Barrier("test-flush") {
			t.Fatal("connection closed")
		}
		select {
		case <-flushed:
		case <-time.After(5 * time.Second):
			t.Fatal("events not handled")
		}
	}
	return svc, srv, flush
}

func params(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestScreenshotDecodesTheImage(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	png := []byte("\x89PNG\r\n\x1a\nfake")
	var got map[string]any
	srv.Handle("Page.captureScreenshot", func(r cdptest.Request) (any, *cdptest.Error) {
		got = params(t, r.Params)
		return map[string]any{"data": base64.StdEncoding.EncodeToString(png)}, nil
	})
	img, err := svc.Screenshot(context.Background(), "jpeg", 80)
	if err != nil || !bytes.Equal(img.Data, png) || img.Format != "jpeg" {
		t.Fatalf("Screenshot = %+v, %v", img, err)
	}
	if got["format"] != "jpeg" || got["quality"] != float64(80) {
		t.Fatalf("params = %v", got)
	}

	srv.Handle("Page.captureScreenshot", func(cdptest.Request) (any, *cdptest.Error) {
		return nil, &cdptest.Error{Code: -32603, Message: "Failed to capture screenshot"}
	})
	if _, err := svc.Screenshot(context.Background(), "", 0); err == nil || !strings.Contains(err.Error(), "Failed to capture screenshot") {
		t.Fatalf("err = %v", err)
	}
}

func TestScreenshotReportsAnAppWithoutScreenshotSupport(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	srv.Handle("Page.captureScreenshot", func(cdptest.Request) (any, *cdptest.Error) {
		return nil, &cdptest.Error{Code: -32601, Message: "Unsupported method 'Page.captureScreenshot'"}
	})
	_, err := svc.Screenshot(context.Background(), "png", 0)
	if !errors.Is(err, perf.ErrScreenshotUnsupported) {
		t.Fatalf("err = %v, want ErrScreenshotUnsupported", err)
	}
	if strings.Contains(err.Error(), "-32601") {
		t.Fatalf("err = %q leaks the protocol code", err)
	}
}

// traceTarget answers Tracing.start/end and, after Tracing.end, streams
// chunks then tracingComplete, as React Native does.
func traceTarget(t *testing.T, srv *cdptest.Server, chunks [][]map[string]any, dataLoss bool) *[]map[string]any {
	t.Helper()
	var mu sync.Mutex
	starts := []map[string]any{}
	srv.Handle("Tracing.start", func(r cdptest.Request) (any, *cdptest.Error) {
		mu.Lock()
		starts = append(starts, params(t, r.Params))
		mu.Unlock()
		return map[string]any{}, nil
	})
	srv.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) {
		go func() {
			time.Sleep(20 * time.Millisecond)
			for _, c := range chunks {
				srv.Emit("Tracing.dataCollected", map[string]any{"value": c})
			}
			srv.Emit("Tracing.tracingComplete", map[string]any{"dataLossOccurred": dataLoss})
		}()
		return map[string]any{}, nil
	})
	return &starts
}

func TestTraceCollectsEveryChunkUntilComplete(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	starts := traceTarget(t, srv, [][]map[string]any{
		{{"name": "a", "ph": "I", "ts": 1}, {"name": "b", "ph": "I", "ts": 2}},
		{{"name": "c", "ph": "X", "ts": 3, "dur": 5}},
	}, false)
	ctx := context.Background()

	if err := svc.StartTrace(ctx, []string{"devtools.timeline", "blink.user_timing"}); err != nil {
		t.Fatal(err)
	}
	if !svc.Status().Tracing {
		t.Fatal("Status does not report the recording")
	}
	if err := svc.StartTrace(ctx, nil); !errors.Is(err, perf.ErrBusy) {
		t.Fatalf("second StartTrace err = %v, want ErrBusy", err)
	}
	trace, err := svc.StopTrace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Events) != 3 || trace.DataLoss {
		t.Fatalf("trace = %d events, loss %v", len(trace.Events), trace.DataLoss)
	}
	if (*starts)[0]["categories"] != "devtools.timeline,blink.user_timing" {
		t.Fatalf("Tracing.start params = %v", (*starts)[0])
	}
	if svc.Status().Tracing {
		t.Fatal("still tracing after StopTrace")
	}

	var buf bytes.Buffer
	if err := perf.WriteTrace(&buf, trace); err != nil {
		t.Fatal(err)
	}
	var file struct {
		TraceEvents []map[string]any `json:"traceEvents"`
	}
	if err := json.Unmarshal(buf.Bytes(), &file); err != nil || len(file.TraceEvents) != 3 || file.TraceEvents[2]["name"] != "c" {
		t.Fatalf("trace file = %s (%v)", buf.String(), err)
	}
}

func TestTraceErrorsAndDefaults(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{TraceTimeout: 100 * time.Millisecond})
	ctx := context.Background()
	if _, err := svc.StopTrace(ctx); !errors.Is(err, perf.ErrNotRecording) {
		t.Fatalf("StopTrace without start err = %v", err)
	}
	starts := traceTarget(t, srv, nil, true)
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if (*starts)[0]["categories"] != strings.Join(perf.DefaultTraceCategories, ",") {
		t.Fatalf("default categories = %v", (*starts)[0])
	}
	trace, err := svc.StopTrace(ctx)
	if err != nil || !trace.DataLoss {
		t.Fatalf("trace = %+v, %v; want data loss reported", trace, err)
	}

	// No tracingComplete: StopTrace gives up after TraceTimeout with an error.
	srv.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopTrace(ctx); !errors.Is(err, perf.ErrTraceIncomplete) {
		t.Fatalf("StopTrace without completion err = %v, want ErrTraceIncomplete", err)
	}
	if svc.Status().Tracing {
		t.Fatal("a failed stop left the recording running")
	}

	srv.Handle("Tracing.start", func(cdptest.Request) (any, *cdptest.Error) {
		return nil, &cdptest.Error{Code: -32603, Message: "The Tracing domain is unavailable when multiple React Native hosts are registered."}
	})
	if err := svc.StartTrace(ctx, nil); err == nil || svc.Status().Tracing {
		t.Fatalf("StartTrace refused by the app err = %v, tracing %v", err, svc.Status().Tracing)
	}
}

func TestCPUProfile(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	srv.Handle("Profiler.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	srv.Handle("Profiler.stop", func(cdptest.Request) (any, *cdptest.Error) {
		return map[string]any{"profile": map[string]any{"nodes": []any{map[string]any{"id": 1}}, "startTime": 1, "endTime": 2}}, nil
	})
	ctx := context.Background()
	if _, err := svc.StopProfile(ctx); !errors.Is(err, perf.ErrNotRecording) {
		t.Fatalf("StopProfile without start err = %v", err)
	}
	if err := svc.StartProfile(ctx); err != nil || !svc.Status().Profiling {
		t.Fatalf("StartProfile err = %v, profiling %v", err, svc.Status().Profiling)
	}
	profile, err := svc.StopProfile(ctx)
	if err != nil || params(t, profile)["nodes"] == nil {
		t.Fatalf("profile = %s, %v", profile, err)
	}
}

func TestHeapSnapshotStreamsEveryChunkInOrder(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	var want strings.Builder
	srv.Handle("HeapProfiler.takeHeapSnapshot", func(cdptest.Request) (any, *cdptest.Error) {
		for i := range 200 {
			chunk := strings.Repeat(string(rune('a'+i%26)), 500)
			want.WriteString(chunk)
			srv.Emit("HeapProfiler.addHeapSnapshotChunk", map[string]any{"chunk": chunk})
		}
		return map[string]any{}, nil
	})
	var buf bytes.Buffer
	n, err := svc.HeapSnapshot(context.Background(), &buf)
	if err != nil || n != int64(buf.Len()) || buf.String() != want.String() {
		t.Fatalf("snapshot = %d bytes (%d written), %v; complete: %v", buf.Len(), n, err, buf.String() == want.String())
	}
}

func TestANewSessionEndsRecordings(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	traceTarget(t, srv, nil, false)
	srv.Handle("Profiler.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	ctx := context.Background()
	_ = svc.StartTrace(ctx, nil)
	_ = svc.StartProfile(ctx)
	svc.BeginSession()
	if st := svc.Status(); st.Tracing || st.Profiling {
		t.Fatalf("status after a new session = %+v", st)
	}
}

func TestANewSessionReleasesAWaitingStop(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	srv.Handle("Tracing.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	srv.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	ctx := context.Background()
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { _, err := svc.StopTrace(ctx); errc <- err }()
	time.Sleep(50 * time.Millisecond)
	svc.BeginSession()
	select {
	case err := <-errc:
		if !errors.Is(err, perf.ErrSessionEnded) {
			t.Fatalf("err = %v, want ErrSessionEnded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopTrace still waiting after a new session")
	}
}

// React Native sends trace chunks only after Tracing.end; late chunks of a
// timed-out recording must not join the next one.
func TestStopTraceReportsTheSessionEndingWhenTheConnectionDrops(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	srv.Handle("Tracing.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	srv.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) {
		srv.DropAll() // the app goes away before answering, as on a detach
		return map[string]any{}, nil
	})
	ctx := context.Background()
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopTrace(ctx); !errors.Is(err, perf.ErrSessionEnded) {
		t.Fatalf("StopTrace = %v, want ErrSessionEnded", err)
	}
	if svc.Status().Tracing {
		t.Fatal("still tracing after the connection dropped")
	}
}

func TestLateChunksOfATimedOutTraceStayOut(t *testing.T) {
	t.Parallel()
	// The first trace must time out; the second must not, although its chunks
	// come 20ms after Tracing.end. 50ms was too tight for loaded CI runners.
	svc, srv, flush := newServiceWithFlush(t, perf.Options{TraceTimeout: 500 * time.Millisecond})
	srv.Handle("Tracing.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	srv.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	ctx := context.Background()
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StopTrace(ctx); !errors.Is(err, perf.ErrTraceIncomplete) {
		t.Fatalf("err = %v", err)
	}
	if err := svc.StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	srv.Emit("Tracing.dataCollected", map[string]any{"value": []map[string]any{{"name": "stale"}}})
	flush()
	traceTarget(t, srv, [][]map[string]any{{{"name": "fresh", "ph": "I", "ts": 1}}}, false)
	trace, err := svc.StopTrace(ctx)
	if err != nil || len(trace.Events) != 1 || !strings.Contains(string(trace.Events[0]), "fresh") {
		t.Fatalf("trace = %d events (%v), want only the fresh one", len(trace.Events), err)
	}
}

func TestConcurrentStopProfileSendsOneStop(t *testing.T) {
	t.Parallel()
	svc, srv := newService(t, perf.Options{})
	srv.Handle("Profiler.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	srv.Handle("Profiler.stop", func(cdptest.Request) (any, *cdptest.Error) {
		time.Sleep(300 * time.Millisecond) // the second stop arrives meanwhile
		return map[string]any{"profile": map[string]any{"nodes": []any{map[string]any{"id": 1}}}}, nil
	})
	ctx := context.Background()
	if err := svc.StartProfile(ctx); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { _, err := svc.StopProfile(ctx); first <- err }()
	for !slices.Contains(srv.Methods(), "Profiler.stop") {
		time.Sleep(time.Millisecond)
	}
	if _, err := svc.StopProfile(ctx); !errors.Is(err, perf.ErrBusy) {
		t.Fatalf("second StopProfile err = %v, want ErrBusy", err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.Join(srv.Methods(), " "), "Profiler.stop"); n != 1 {
		t.Fatalf("Profiler.stop sent %d times", n)
	}
}
