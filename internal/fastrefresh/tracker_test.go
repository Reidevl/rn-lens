package fastrefresh_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/fastrefresh"
)

const bundle = "http://192.168.1.5:8081/index.bundle?platform=ios&dev=true"

// evaluator answers Runtime.evaluate with the bundle URL, like Hermes does
// for nativeModuleProxy.SourceCode.getConstants().scriptURL.
type evaluator struct{ url string }

func (e evaluator) Call(_ context.Context, method string, _ any) (json.RawMessage, error) {
	if method != "Runtime.evaluate" {
		return nil, errors.New("unexpected " + method)
	}
	if e.url == "" {
		return json.RawMessage(`{"result":{"type":"undefined"}}`), nil
	}
	v, _ := json.Marshal(e.url)
	return json.RawMessage(`{"result":{"type":"string","value":` + string(v) + `}}`), nil
}

// stream is an in-memory Metro /hot socket.
type stream struct {
	in     chan []byte
	sent   chan []byte
	closed chan struct{}
	once   sync.Once
}

func (s *stream) Send(_ context.Context, v any) error {
	b, _ := json.Marshal(v)
	s.sent <- b
	return nil
}

func (s *stream) Recv(ctx context.Context) ([]byte, error) {
	select {
	case b := <-s.in:
		return b, nil
	case <-s.closed:
		return nil, errors.New("closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *stream) Close() error { s.once.Do(func() { close(s.closed) }); return nil }

type dialer struct {
	urls    chan string
	streams chan *stream
}

func newDialer() *dialer { return &dialer{urls: make(chan string, 8), streams: make(chan *stream, 8)} }

func (d *dialer) DialHMR(_ context.Context, url string) (fastrefresh.Stream, error) {
	s := &stream{in: make(chan []byte, 16), sent: make(chan []byte, 16), closed: make(chan struct{})}
	d.urls <- url
	d.streams <- s
	return s, nil
}

func next[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
		var zero T
		return zero
	}
}

func waitState(t *testing.T, tr *fastrefresh.Tracker, ok func(fastrefresh.State) bool) fastrefresh.State {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := tr.State(); ok(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state never matched, last %+v", tr.State())
	return fastrefresh.State{}
}

func phase(p fastrefresh.Phase) func(fastrefresh.State) bool {
	return func(s fastrefresh.State) bool { return s.Phase == p }
}

func TestTrackerRegistersAndReportsUpdates(t *testing.T) {
	t.Parallel()
	d := newDialer()
	tr := fastrefresh.New(d, evaluator{url: bundle}, fastrefresh.Options{})
	t.Cleanup(tr.Stop)

	tr.Attached("http://127.0.0.1:8081")
	if got := next(t, d.urls); got != "ws://127.0.0.1:8081/hot" {
		t.Fatalf("dialed %q", got)
	}
	s := next(t, d.streams)
	var reg struct {
		Type        string   `json:"type"`
		EntryPoints []string `json:"entryPoints"`
	}
	if err := json.Unmarshal(next(t, s.sent), &reg); err != nil || reg.Type != "register-entrypoints" || len(reg.EntryPoints) != 1 || reg.EntryPoints[0] != bundle {
		t.Fatalf("registration = %+v (%v)", reg, err)
	}

	// The initial update after registering is not a Fast Refresh.
	s.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":true}}`)
	s.in <- []byte(`{"type":"update","body":{"revisionId":"1","isInitialUpdate":true,"added":[],"modified":[],"deleted":[]}}`)
	s.in <- []byte(`{"type":"update-done"}`)
	s.in <- []byte(`{"type":"bundle-registered"}`)
	waitState(t, tr, phase(fastrefresh.PhaseReady))

	s.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":false}}`)
	waitState(t, tr, phase(fastrefresh.PhaseUpdating))
	s.in <- []byte(`{"type":"update","body":{"revisionId":"2","isInitialUpdate":false,"added":[[1,"a"]],"modified":[[2,"b"],[3,"c"]],"deleted":[4]}}`)
	s.in <- []byte(`{"type":"update-done"}`)
	st := waitState(t, tr, phase(fastrefresh.PhaseUpdated))
	if st.Modules != 4 || st.At.IsZero() {
		t.Fatalf("updated state = %+v", st)
	}

	s.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":false}}`)
	s.in <- []byte(`{"type":"error","body":{"type":"TransformError","message":"SyntaxError: App.tsx: Unexpected token (3:4)","errors":[]}}`)
	st = waitState(t, tr, phase(fastrefresh.PhaseError))
	if st.Message != "SyntaxError: App.tsx: Unexpected token (3:4)" {
		t.Fatalf("error state = %+v", st)
	}
}

func TestTrackerReconnectsAndStopsOnDetach(t *testing.T) {
	t.Parallel()
	d := newDialer()
	tr := fastrefresh.New(d, evaluator{url: bundle}, fastrefresh.Options{BackoffMin: time.Millisecond, BackoffMax: 5 * time.Millisecond})
	t.Cleanup(tr.Stop)
	tr.Attached("https://metro.local:8443")
	if got := next(t, d.urls); got != "wss://metro.local:8443/hot" {
		t.Fatalf("dialed %q", got)
	}
	first := next(t, d.streams)
	next(t, first.sent)
	waitState(t, tr, phase(fastrefresh.PhaseReady))

	_ = first.Close() // Metro restarted
	next(t, d.urls)
	second := next(t, d.streams)
	next(t, second.sent)
	waitState(t, tr, phase(fastrefresh.PhaseReady))

	tr.Detached()
	if st := tr.State(); st.Phase != fastrefresh.PhaseOff {
		t.Fatalf("state after detach = %+v", st)
	}
	select {
	case <-second.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Detached left the socket open")
	}
}

func TestTrackerWithoutABundleURL(t *testing.T) {
	t.Parallel()
	d := newDialer()
	tr := fastrefresh.New(d, evaluator{}, fastrefresh.Options{})
	t.Cleanup(tr.Stop)
	tr.Attached("http://127.0.0.1:8081")
	st := waitState(t, tr, phase(fastrefresh.PhaseUnavailable))
	if st.Message == "" {
		t.Fatal("unavailable without a reason")
	}
	select {
	case u := <-d.urls:
		t.Fatalf("dialed %q without a bundle URL", u)
	default:
	}
}

// After a Metro restart the app's bundle graph is gone until the app
// reloads: Metro answers the registration with GraphNotFoundError.
func TestTrackerWaitsForTheAppToReloadAfterAMetroRestart(t *testing.T) {
	t.Parallel()
	d := newDialer()
	tr := fastrefresh.New(d, evaluator{url: bundle}, fastrefresh.Options{BackoffMin: time.Millisecond, BackoffMax: 5 * time.Millisecond})
	t.Cleanup(tr.Stop)
	tr.Attached("http://127.0.0.1:8081")
	next(t, d.urls)
	first := next(t, d.streams)
	next(t, first.sent)
	first.in <- []byte(`{"type":"error","body":{"type":"GraphNotFoundError","message":"The graph ` + "`{...}`" + ` was not found."}}`)
	st := waitState(t, tr, phase(fastrefresh.PhaseUnavailable))
	if st.Message != "Reload the app to resume Fast Refresh: Metro has not built its bundle since it restarted." {
		t.Fatalf("state = %+v", st)
	}
	// Lens registers again until Metro knows the bundle.
	next(t, d.urls)
	second := next(t, d.streams)
	next(t, second.sent)
	if st := tr.State(); st.Phase != fastrefresh.PhaseUnavailable {
		t.Fatalf("the hint went away before Metro answered: %+v", st)
	}
	second.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":true}}`)
	waitState(t, tr, phase(fastrefresh.PhaseReady))
}

// Metro sends update-start, then update or error, then update-done for
// every rebuild: update-done must not hide the error.
func TestBundlingErrorSurvivesUpdateDone(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var phases []fastrefresh.Phase
	d := newDialer()
	var tr *fastrefresh.Tracker
	tr = fastrefresh.New(d, evaluator{url: bundle}, fastrefresh.Options{OnChange: func() {
		mu.Lock()
		phases = append(phases, tr.State().Phase)
		mu.Unlock()
	}})
	t.Cleanup(tr.Stop)
	tr.Attached("http://127.0.0.1:8081")
	s := next(t, d.streams)
	next(t, s.sent)
	waitState(t, tr, phase(fastrefresh.PhaseReady))
	s.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":false}}`)
	s.in <- []byte(`{"type":"error","body":{"type":"TransformError","message":"SyntaxError: boom","errors":[]}}`)
	s.in <- []byte(`{"type":"update-done","body":{}}`)
	// The next rebuild proves update-done was handled before it.
	s.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":false}}`)
	waitState(t, tr, func(st fastrefresh.State) bool {
		mu.Lock()
		defer mu.Unlock()
		return len(phases) > 0 && phases[len(phases)-1] == fastrefresh.PhaseUpdating && st.Phase == fastrefresh.PhaseUpdating
	})
	mu.Lock()
	defer mu.Unlock()
	for i, p := range phases {
		if p == fastrefresh.PhaseError {
			if rest := phases[i+1:]; len(rest) != 1 || rest[0] != fastrefresh.PhaseUpdating {
				t.Fatalf("phases after the error = %v, want only updating", rest)
			}
			return
		}
	}
	t.Fatalf("phases = %v, no error", phases)
}

// A registration that was replaced or stopped never changes the state.
func TestAStaleRegistrationCannotChangeTheState(t *testing.T) {
	t.Parallel()
	d := newDialer()
	tr := fastrefresh.New(d, evaluator{url: bundle}, fastrefresh.Options{})
	t.Cleanup(tr.Stop)
	tr.Attached("http://127.0.0.1:8081")
	old := next(t, d.streams)
	next(t, old.sent)
	waitState(t, tr, phase(fastrefresh.PhaseReady))
	tr.Detached()
	old.in <- []byte(`{"type":"update-start","body":{"isInitialUpdate":false}}`)
	old.in <- []byte(`{"type":"error","body":{"type":"TransformError","message":"late"}}`)
	time.Sleep(50 * time.Millisecond)
	if st := tr.State(); st.Phase != fastrefresh.PhaseOff {
		t.Fatalf("state after a late message of a stopped registration = %+v", st)
	}
}
