package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/platform/bus"
	"github.com/Reidevl/rn-lens/internal/targets"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         app.Config
		wantVersion string
	}{
		{name: "reports the configured version", cfg: app.Config{Version: "1.2.3"}, wantVersion: "1.2.3"},
		{name: "defaults an empty version to dev", cfg: app.Config{}, wantVersion: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := app.New(tt.cfg).Info()

			if got.Name != "Lens" {
				t.Errorf("Info().Name = %q, want %q", got.Name, "Lens")
			}
			if got.Version != tt.wantVersion {
				t.Errorf("Info().Version = %q, want %q", got.Version, tt.wantVersion)
			}
		})
	}
}

// fakeMetro lists one Fusebox page whose debugger socket is a cdptest server.
type fakeMetro struct {
	srv    *httptest.Server
	mu     sync.Mutex
	pages  []metro.Page
	opened []string
}

func newFakeMetro(t *testing.T, debugger *cdptest.Server) *fakeMetro {
	t.Helper()
	f := &fakeMetro{}
	f.pages = []metro.Page{{
		ID:                   "dev-1",
		Title:                "React Native",
		AppID:                "com.example.app",
		WebSocketDebuggerURL: debugger.URL(),
		ReactNative:          metro.ReactNativeInfo{LogicalDeviceID: "dev", Capabilities: metro.Capabilities{NativePageReloads: true}},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /json", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.pages)
	})
	mux.HandleFunc("POST /open-debugger", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.opened = append(f.opened, r.URL.Query().Get("target"))
		w.WriteHeader(http.StatusOK)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func waitTopic(t *testing.T, ch <-chan bus.Message, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestDiscoverAttachReloadAndHandOff(t *testing.T) {
	t.Parallel()
	debugger := cdptest.New(t)
	reloads := make(chan struct{}, 1)
	debugger.Handle("Page.reload", func(cdptest.Request) (any, *cdptest.Error) {
		reloads <- struct{}{}
		return nil, nil
	})
	m := newFakeMetro(t, debugger)

	a := app.New(app.Config{MetroEndpoints: []string{m.srv.URL}, PollInterval: 10 * time.Millisecond})
	targetsCh, cancelTargets := a.Bus().Subscribe(app.TopicTargetsChanged, 8)
	defer cancelTargets()
	sessionCh, cancelSession := a.Bus().Subscribe(app.TopicSessionChanged, 64)
	defer cancelSession()
	a.Start()
	t.Cleanup(a.Stop)

	waitTopic(t, targetsCh, "targets:changed")
	list := a.Targets()
	if len(list) != 1 || list[0].AppID != "com.example.app" || !list[0].Reloadable {
		t.Fatalf("Targets = %+v", list)
	}

	if err := a.Attach("nope"); !errors.Is(err, app.ErrUnknownTarget) {
		t.Fatalf("Attach(unknown) = %v, want ErrUnknownTarget", err)
	}
	if err := a.Attach(list[0].ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for a.SessionSnapshot().State != targets.StateAttached {
		waitTopic(t, sessionCh, "session attached")
	}

	if err := a.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	select {
	case <-reloads:
	case <-time.After(2 * time.Second):
		t.Fatal("Page.reload never reached the target")
	}

	if err := a.OpenInReactNativeDevTools(context.Background(), list[0].ID); err != nil {
		t.Fatalf("OpenInReactNativeDevTools: %v", err)
	}
	m.mu.Lock()
	if len(m.opened) != 1 || m.opened[0] != "dev-1" {
		t.Fatalf("open-debugger targets = %v, want [dev-1]", m.opened)
	}
	m.mu.Unlock()

	a.Detach()
	if got := a.SessionSnapshot().State; got != targets.StateClosed {
		t.Fatalf("state after Detach = %s", got)
	}
}

func TestSessionFollowsTheAppToANewPageId(t *testing.T) {
	t.Parallel()
	first := cdptest.New(t)
	second := cdptest.New(t)
	m := newFakeMetro(t, first)

	a := app.New(app.Config{MetroEndpoints: []string{m.srv.URL}, PollInterval: 10 * time.Millisecond})
	sessionCh, cancelSession := a.Bus().Subscribe(app.TopicSessionChanged, 64)
	defer cancelSession()
	targetsCh, cancelTargets := a.Bus().Subscribe(app.TopicTargetsChanged, 8)
	defer cancelTargets()
	a.Start()
	t.Cleanup(a.Stop)

	waitTopic(t, targetsCh, "targets:changed")
	if err := a.Attach(a.Targets()[0].ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for a.SessionSnapshot().State != targets.StateAttached {
		waitTopic(t, sessionCh, "attached")
	}

	// The app restarts: Metro now lists it under page 2 with a new socket.
	m.mu.Lock()
	m.pages = []metro.Page{{
		ID:                   "dev-2",
		Title:                "React Native",
		AppID:                "com.example.app",
		WebSocketDebuggerURL: second.URL(),
		ReactNative:          metro.ReactNativeInfo{LogicalDeviceID: "dev", Capabilities: metro.Capabilities{NativePageReloads: true}},
	}}
	m.mu.Unlock()
	first.CloseAll(1000, "[PAGE_REMOVED] Page removed")

	deadline := time.After(5 * time.Second)
	for {
		snap := a.SessionSnapshot()
		if snap.State == targets.StateAttached && snap.Target != nil && snap.Target.MetroID == "dev-2" {
			break
		}
		select {
		case <-sessionCh:
		case <-targetsCh:
		case <-deadline:
			t.Fatalf("session did not follow the app to dev-2; last %+v", snap)
		}
	}
	if second.ConnectionCount() != 1 {
		t.Fatalf("second debugger saw %d connections, want 1", second.ConnectionCount())
	}
}

func TestStopIsIdempotentAndStopsDiscovery(t *testing.T) {
	t.Parallel()
	a := app.New(app.Config{MetroEndpoints: []string{"http://127.0.0.1:1"}, PollInterval: 5 * time.Millisecond})
	a.Start()
	a.Stop()
	a.Stop()
	if got := a.SessionSnapshot().State; got != targets.StateIdle {
		t.Fatalf("state = %s, want idle", got)
	}
}

func TestOriginFor(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"ws://localhost:8081/inspector/debug?device=1&page=1", "http://127.0.0.1:8081"},
		{"ws://127.0.0.1:8082/inspector/debug", "http://127.0.0.1:8082"},
		{"ws://[::1]:8081/inspector/debug", "http://127.0.0.1:8081"},
		{"wss://tunnel.example.dev/inspector/debug", "https://tunnel.example.dev"},
		{"ws://192.168.1.10:19000/x", "http://192.168.1.10:19000"},
		{"not a url", ""},
	}
	for _, tt := range tests {
		if got := app.OriginFor(tt.in); got != tt.want {
			t.Errorf("OriginFor(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
