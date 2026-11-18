package targets_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// wsDialer adapts cdpws to the targets.Dialer port, as the composition root does.
var wsDialer = targets.DialerFunc(func(ctx context.Context, url string) (targets.Conn, error) {
	return cdpws.Dial(ctx, url, cdpws.Options{})
})

type stateRecorder struct {
	mu     sync.Mutex
	states []targets.Snapshot
	notify chan struct{}
}

func newStateRecorder() *stateRecorder {
	return &stateRecorder{notify: make(chan struct{}, 128)}
}

func (r *stateRecorder) onChange(s targets.Snapshot) {
	r.mu.Lock()
	r.states = append(r.states, s)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

// waitFor blocks until pred holds for the latest snapshot.
func (r *stateRecorder) waitFor(t *testing.T, what string, pred func(targets.Snapshot) bool) targets.Snapshot {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		r.mu.Lock()
		if n := len(r.states); n > 0 && pred(r.states[n-1]) {
			s := r.states[n-1]
			r.mu.Unlock()
			return s
		}
		r.mu.Unlock()
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("timed out waiting for %s; states: %v", what, r.sequence())
		}
	}
}

func (r *stateRecorder) sequence() []targets.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]targets.State, len(r.states))
	for i, s := range r.states {
		out[i] = s.State
	}
	return out
}

// sawBefore reports whether a snapshot matching first was recorded before the
// earliest snapshot matching second. It is false when second never happened.
func (r *stateRecorder) sawBefore(first, second func(targets.Snapshot) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.states {
		if second(s) {
			return false
		}
		if first(s) {
			return true
		}
	}
	return false
}

// targetSequence lists the state and target id of every recorded snapshot.
func (r *stateRecorder) targetSequence() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.states))
	for _, s := range r.states {
		id := "<nil>"
		if s.Target != nil {
			id = s.Target.ID
		}
		out = append(out, string(s.State)+":"+id)
	}
	return out
}

func inState(state targets.State) func(targets.Snapshot) bool {
	return func(s targets.Snapshot) bool { return s.State == state }
}

// targetFor builds the Target the composition root would derive from a
// Fusebox page served by srv.
func targetFor(srv *cdptest.Server) targets.Target {
	return targets.Target{
		ID:              "http://localhost:8081|dev-1",
		Endpoint:        "http://localhost:8081",
		MetroID:         "dev-1",
		PageID:          "1",
		Title:           "React Native",
		AppID:           "com.example.app",
		LogicalDeviceID: "dev",
		WebSocketURL:    srv.URL(),
		Capabilities:    targets.Capabilities{NativePageReloads: true, SupportsMultipleDebuggers: true},
		Reloadable:      true,
	}
}

func newSession(t *testing.T, rec *stateRecorder) *targets.Session {
	t.Helper()
	s := targets.NewSession(wsDialer, targets.SessionOptions{
		BackoffMin: 5 * time.Millisecond,
		BackoffMax: 20 * time.Millisecond,
		OnChange:   rec.onChange,
	})
	t.Cleanup(s.Detach)
	return s
}

func TestAttachEnablesApplicationDomainFirstAndReportsMetadata(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle(cdp.MethodReactNativeApplicationEnable, func(cdptest.Request) (any, *cdptest.Error) {
		go srv.Emit(cdp.EventReactNativeMetadataUpdated, map[string]any{
			"appDisplayName":                    "Example",
			"appIdentifier":                     "com.example.app",
			"deviceName":                        "iPhone 17",
			"integrationName":                   "React Native Bridgeless (Hermes)",
			"platform":                          "ios",
			"reactNativeVersion":                "0.88.1",
			"unstable_networkInspectionEnabled": true,
		})
		return nil, nil
	})
	rec := newStateRecorder()
	s := newSession(t, rec)

	s.Attach(targetFor(srv))

	snap := rec.waitFor(t, "metadata", func(s targets.Snapshot) bool { return s.State == targets.StateAttached && s.Metadata != nil })
	if snap.Metadata.AppDisplayName != "Example" || snap.Metadata.ReactNativeVersion != "0.88.1" || !snap.Metadata.NetworkInspectionEnabled {
		t.Fatalf("metadata = %+v", snap.Metadata)
	}
	if methods := srv.Methods(); len(methods) == 0 || methods[0] != cdp.MethodReactNativeApplicationEnable {
		t.Fatalf("first method = %v, want ReactNativeApplication.enable", methods)
	}
	if got := s.Snapshot().Target.AppID; got != "com.example.app" {
		t.Fatalf("Target.AppID = %q", got)
	}
}

func TestEvictionStopsWithoutReconnecting(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)
	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))

	srv.CloseAll(websocket.StatusNormalClosure, "[NEW_DEBUGGER_OPENED] New debugger opened for the same app instance")

	snap := rec.waitFor(t, "evicted", inState(targets.StateEvicted))
	if snap.Reason == "" || snap.Metadata != nil {
		t.Fatalf("evicted snapshot = %+v", snap)
	}
	time.Sleep(50 * time.Millisecond) // longer than BackoffMax: a reconnect would have happened
	if n := srv.ConnectionCount(); n != 1 {
		t.Fatalf("server accepted %d connections, want 1 (no reconnect after eviction)", n)
	}
	if _, err := s.Call(context.Background(), "Runtime.enable", nil); !errors.Is(err, targets.ErrNotAttached) {
		t.Fatalf("Call after eviction = %v, want ErrNotAttached", err)
	}
}

func TestConnectionLossReconnectsWithBackoff(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)
	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	srv.WaitConnected(time.Second)

	srv.CloseAll(websocket.StatusNormalClosure, "[CONNECTION_LOST] Connection lost")

	rec.waitFor(t, "reconnecting", inState(targets.StateReconnecting))
	rec.waitFor(t, "re-attached", inState(targets.StateAttached))

	seq := rec.sequence()
	sawReconnect := false
	for _, st := range seq {
		if st == targets.StateReconnecting {
			sawReconnect = true
		}
	}
	if !sawReconnect || seq[len(seq)-1] != targets.StateAttached {
		t.Fatalf("state sequence = %v", seq)
	}
	if enables := countMethod(srv, cdp.MethodReactNativeApplicationEnable); enables != 2 {
		t.Fatalf("enable sent %d times, want 2 (once per connection)", enables)
	}
}

func TestDialFailureKeepsRetryingUntilDetach(t *testing.T) {
	t.Parallel()
	rec := newStateRecorder()
	s := newSession(t, rec)
	target := targets.Target{ID: "x", WebSocketURL: "ws://127.0.0.1:1/inspector/debug?device=1&page=1"}

	s.Attach(target)

	snap := rec.waitFor(t, "second retry", func(s targets.Snapshot) bool {
		return s.State == targets.StateReconnecting && s.Attempts >= 2
	})
	if snap.Reason == "" {
		t.Fatal("reconnecting snapshot has no reason")
	}

	s.Detach()

	if got := s.Snapshot().State; got != targets.StateClosed {
		t.Fatalf("state after Detach = %s, want closed", got)
	}
}

func TestDetachClosesAndReloadNeedsAttachment(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	reloads := 0
	var mu sync.Mutex
	srv.Handle(cdp.MethodPageReload, func(cdptest.Request) (any, *cdptest.Error) {
		mu.Lock()
		reloads++
		mu.Unlock()
		return nil, nil
	})
	rec := newStateRecorder()
	s := newSession(t, rec)

	if err := s.Reload(context.Background()); !errors.Is(err, targets.ErrNotAttached) {
		t.Fatalf("Reload while idle = %v, want ErrNotAttached", err)
	}

	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	if err := s.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	mu.Lock()
	if reloads != 1 {
		t.Fatalf("Page.reload sent %d times, want 1", reloads)
	}
	mu.Unlock()

	s.Detach()
	snap := s.Snapshot()
	if snap.State != targets.StateClosed || snap.Metadata != nil {
		t.Fatalf("snapshot after Detach = %+v", snap)
	}
}

func TestEventHandlersReceiveEventsInOrder(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)
	var mu sync.Mutex
	var seen []string
	s.OnEvent(func(ev cdp.Event) {
		mu.Lock()
		seen = append(seen, ev.Method)
		mu.Unlock()
	})
	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	srv.WaitConnected(time.Second)

	srv.Emit("Runtime.consoleAPICalled", map[string]any{"n": 1})
	srv.Emit("Log.entryAdded", map[string]any{"n": 2})

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		if len(seen) == 2 {
			if seen[0] != "Runtime.consoleAPICalled" || seen[1] != "Log.entryAdded" {
				t.Fatalf("events = %v", seen)
			}
			mu.Unlock()
			return
		}
		mu.Unlock()
		select {
		case <-deadline:
			t.Fatal("handlers did not receive both events")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestAttachReplacesPreviousSession(t *testing.T) {
	t.Parallel()
	first := cdptest.New(t)
	second := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)

	s.Attach(targetFor(first))
	rec.waitFor(t, "first attached", inState(targets.StateAttached))
	s.Attach(targetFor(second))
	rec.waitFor(t, "second attached", func(snap targets.Snapshot) bool {
		return snap.State == targets.StateAttached && snap.Target != nil && snap.Target.WebSocketURL == second.URL()
	})

	if n := countMethod(second, cdp.MethodReactNativeApplicationEnable); n != 1 {
		t.Fatalf("second server saw %d enables, want 1", n)
	}
}

func countMethod(srv *cdptest.Server, method string) int {
	n := 0
	for _, m := range srv.Methods() {
		if m == method {
			n++
		}
	}
	return n
}

func TestFindReplacementPrefersReloadablePageOfSameApp(t *testing.T) {
	t.Parallel()
	previous := targets.Target{ID: "e|dev-1", Endpoint: "e", AppID: "com.app", LogicalDeviceID: "dev", Reloadable: true}
	other := targets.Target{ID: "e|other-1", Endpoint: "e", AppID: "com.other", LogicalDeviceID: "dev", Reloadable: true}
	legacy := targets.Target{ID: "e|dev-2", Endpoint: "e", AppID: "com.app", LogicalDeviceID: "dev"}
	modern := targets.Target{ID: "e|dev-3", Endpoint: "e", AppID: "com.app", LogicalDeviceID: "dev", Reloadable: true}

	if got, ok := targets.FindReplacement(previous, []targets.Target{other, legacy, modern}); !ok || got.ID != modern.ID {
		t.Fatalf("FindReplacement = %+v, %v; want the modern page", got, ok)
	}
	if got, ok := targets.FindReplacement(previous, []targets.Target{other, legacy}); !ok || got.ID != legacy.ID {
		t.Fatalf("FindReplacement without a modern page = %+v, %v; want the legacy page", got, ok)
	}
	if _, ok := targets.FindReplacement(previous, []targets.Target{previous, other}); ok {
		t.Fatal("FindReplacement must not pick the original page or another app")
	}
	byName := targets.Target{ID: "e|x-1", Endpoint: "e", AppID: "com.app", DeviceName: "Pixel"}
	if _, ok := targets.FindReplacement(targets.Target{ID: "e|y-1", Endpoint: "e", AppID: "com.app", DeviceName: "Pixel"}, []targets.Target{byName}); !ok {
		t.Fatal("FindReplacement should fall back to device name when logical ids are missing")
	}
}

// Two goroutines attach at once (a UI click racing a programmatic attach).
// After Detach returns no run loop may still be alive.
func TestConcurrentAttachLeavesNoLoopBehind(t *testing.T) {
	t.Parallel()
	first := cdptest.New(t)
	second := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.Attach(targetFor(first)) }()
	go func() { defer wg.Done(); s.Attach(targetFor(second)) }()
	wg.Wait()
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	time.Sleep(50 * time.Millisecond)

	s.Detach()
	before := len(rec.sequence())
	// A leaked loop would still be pumping: closing its socket makes it transition.
	first.CloseAll(websocket.StatusGoingAway, "[CONNECTION_LOST] x")
	second.CloseAll(websocket.StatusGoingAway, "[CONNECTION_LOST] x")
	time.Sleep(150 * time.Millisecond)

	after := rec.sequence()
	if len(after) != before || s.Snapshot().State != targets.StateClosed {
		t.Fatalf("transitions after Detach: %v (final %s)", after[before:], s.Snapshot().State)
	}
}

func TestDetachAfterEvictionReportsClosed(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)
	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	srv.CloseAll(websocket.StatusNormalClosure, "[NEW_DEBUGGER_OPENED] New debugger opened")
	rec.waitFor(t, "evicted", inState(targets.StateEvicted))

	s.Detach()

	if got := s.Snapshot().State; got != targets.StateClosed {
		t.Fatalf("state after Detach = %s, want closed", got)
	}
}

func TestResolveRepointsReconnectingSession(t *testing.T) {
	t.Parallel()
	first := cdptest.New(t)
	second := cdptest.New(t)
	rec := newStateRecorder()
	moved := targetFor(second)
	moved.ID = "moved"
	s := targets.NewSession(wsDialer, targets.SessionOptions{
		BackoffMin: 5 * time.Millisecond,
		BackoffMax: 20 * time.Millisecond,
		OnChange:   rec.onChange,
		Resolve: func(current targets.Target) (targets.Target, bool) {
			if current.ID == moved.ID {
				return current, false
			}
			return moved, true
		},
	})
	t.Cleanup(s.Detach)

	s.Attach(targetFor(first))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	first.CloseAll(websocket.StatusGoingAway, "[PAGE_REMOVED] Page removed")

	snap := rec.waitFor(t, "re-attached to the moved page", func(snap targets.Snapshot) bool {
		return snap.State == targets.StateAttached && snap.Target != nil && snap.Target.ID == moved.ID
	})
	if snap.Target.WebSocketURL != second.URL() {
		t.Fatalf("re-attached to %s, want %s", snap.Target.WebSocketURL, second.URL())
	}

	// The UI must learn that the target moved while the session is still
	// reconnecting, before the attached snapshot for the new page arrives.
	if !rec.sawBefore(
		func(s targets.Snapshot) bool {
			return s.State == targets.StateReconnecting && s.Target != nil && s.Target.ID == moved.ID
		},
		func(s targets.Snapshot) bool {
			return s.State == targets.StateAttached && s.Target != nil && s.Target.ID == moved.ID
		},
	) {
		t.Fatalf("no reconnecting snapshot carried the moved target before attached; targets: %v", rec.targetSequence())
	}
}

// A reconnect whose Resolve keeps the same page emits no extra target change:
// the reconnecting and attached snapshots all carry the original target.
func TestResolveKeepingSamePageEmitsNoExtraChange(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	original := targetFor(srv)
	s := targets.NewSession(wsDialer, targets.SessionOptions{
		BackoffMin: 5 * time.Millisecond,
		BackoffMax: 20 * time.Millisecond,
		OnChange:   rec.onChange,
		Resolve: func(current targets.Target) (targets.Target, bool) {
			return current, true
		},
	})
	t.Cleanup(s.Detach)

	s.Attach(original)
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	srv.WaitConnected(time.Second)
	srv.CloseAll(websocket.StatusNormalClosure, "[CONNECTION_LOST] Connection lost")
	rec.waitFor(t, "reconnecting", inState(targets.StateReconnecting))
	rec.waitFor(t, "re-attached", inState(targets.StateAttached))

	rec.mu.Lock()
	defer rec.mu.Unlock()
	firstRetries := 0
	for _, snap := range rec.states {
		if snap.Target == nil || snap.Target.ID != original.ID {
			t.Fatalf("snapshot %s carries target %+v, want %s", snap.State, snap.Target, original.ID)
		}
		if snap.State == targets.StateReconnecting && snap.Attempts == 1 {
			firstRetries++
		}
	}
	if firstRetries != 1 {
		t.Fatalf("first retry emitted %d reconnecting snapshots, want 1 (no extra change for an unchanged page)", firstRetries)
	}
}

func TestBarrierReachesEventHandlers(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	rec := newStateRecorder()
	s := newSession(t, rec)
	if err := s.Barrier("idle"); !errors.Is(err, targets.ErrNotAttached) {
		t.Fatalf("Barrier while idle = %v, want ErrNotAttached", err)
	}
	got := make(chan string, 1)
	s.OnEvent(func(ev cdp.Event) {
		if ev.Method == cdp.EventLensBarrier {
			got <- string(ev.Params)
		}
	})
	s.Attach(targetFor(srv))
	rec.waitFor(t, "attached", inState(targets.StateAttached))
	if err := s.Barrier("b1"); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != `"b1"` {
			t.Fatalf("barrier params = %s", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the barrier never reached the handler")
	}
}
