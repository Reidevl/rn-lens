package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/adapters/sdkws"
	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/perf"
	"github.com/Reidevl/rn-lens/internal/platform/bus"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
	"github.com/Reidevl/rn-lens/internal/storage"
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

func TestAttachEnablesConsoleDomainsAndIngestsEvents(t *testing.T) {
	t.Parallel()
	debugger := cdptest.New(t)
	debugger.Handle("Runtime.enable", func(cdptest.Request) (any, *cdptest.Error) { return nil, nil })
	m := newFakeMetro(t, debugger)
	a := app.New(app.Config{MetroEndpoints: []string{m.srv.URL}, PollInterval: 10 * time.Millisecond})
	targetsCh, cancelTargets := a.Bus().Subscribe(app.TopicTargetsChanged, 8)
	defer cancelTargets()
	consoleCh, cancelConsole := a.Bus().Subscribe(app.TopicConsoleChanged, 64)
	defer cancelConsole()
	a.Start()
	t.Cleanup(a.Stop)

	waitTopic(t, targetsCh, "targets:changed")
	if err := a.Attach(a.Targets()[0].ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		methods := debugger.Methods()
		if len(methods) >= 3 {
			if methods[0] != "ReactNativeApplication.enable" || methods[1] != "Runtime.enable" || methods[2] != "Log.enable" {
				t.Fatalf("methods = %v", methods)
			}
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("domains not enabled; methods = %v", debugger.Methods())
		}
	}

	// Metadata updates re-emit an attached snapshot; they must not re-run the
	// attach sequence (no second Runtime.enable, no "Re-attached" marker).
	debugger.Emit("ReactNativeApplication.metadataUpdated", map[string]any{"appDisplayName": "Example", "platform": "ios", "reactNativeVersion": "0.88.0"})
	debugger.Emit("Runtime.consoleAPICalled", map[string]any{"type": "log", "args": []map[string]any{{"type": "string", "value": "hello"}}, "executionContextId": 1})
	for {
		entries := a.Console().Store().Range(1, 0)
		if len(entries) >= 2 && entries[len(entries)-1].Text == "hello" {
			break
		}
		waitTopic(t, consoleCh, "console:changed")
	}
	time.Sleep(50 * time.Millisecond)
	enables := 0
	for _, m := range debugger.Methods() {
		if m == "Runtime.enable" {
			enables++
		}
	}
	if enables != 1 {
		t.Fatalf("Runtime.enable sent %d times after a metadata update, want 1", enables)
	}
	for _, e := range a.Console().Store().Range(1, 0) {
		if e.Kind == "system" && e.Text != "Attached to React Native" {
			t.Fatalf("unexpected system entry %q", e.Text)
		}
	}
	if got := a.Console().Store().Epoch(); got != 1 {
		t.Fatalf("epoch = %d after a metadata update, want 1", got)
	}
}

func TestAttachEnablesNetworkAndIngestsRequests(t *testing.T) {
	t.Parallel()
	debugger := cdptest.New(t)
	debugger.Handle("Runtime.enable", func(cdptest.Request) (any, *cdptest.Error) { return nil, nil })
	debugger.Handle("Network.enable", func(cdptest.Request) (any, *cdptest.Error) { return nil, nil })
	m := newFakeMetro(t, debugger)
	a := app.New(app.Config{MetroEndpoints: []string{m.srv.URL}, PollInterval: 10 * time.Millisecond, NetworkCapacity: 5})
	targetsCh, cancelTargets := a.Bus().Subscribe(app.TopicTargetsChanged, 8)
	defer cancelTargets()
	networkCh, cancelNetwork := a.Bus().Subscribe(app.TopicNetworkChanged, 64)
	defer cancelNetwork()
	a.Start()
	t.Cleanup(a.Stop)

	waitTopic(t, targetsCh, "targets:changed")
	if err := a.Attach(a.Targets()[0].ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for !a.Network().Availability().Checked {
		waitTopic(t, networkCh, "network availability")
	}
	if got := a.Network().Availability(); !got.Supported {
		t.Fatalf("Availability = %+v", got)
	}

	debugger.Emit("Network.requestWillBeSent", map[string]any{
		"requestId": "1", "timestamp": 1, "request": map[string]any{"url": "https://httpbin.org/json", "method": "GET", "headers": map[string]any{}},
	})
	for a.Network().Store().Stats().Len == 0 {
		waitTopic(t, networkCh, "network:changed")
	}
	if rows := a.Network().Store().Since(0, 0); rows[0].URL != "https://httpbin.org/json" {
		t.Fatalf("rows = %+v", rows)
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

func TestSDKServerFeedsTheQueriesFeature(t *testing.T) {
	t.Parallel()
	a := app.New(app.Config{MetroEndpoints: []string{"http://127.0.0.1:1"}, PollInterval: time.Hour, SDKAddr: "127.0.0.1:0"})
	queriesCh, cancelQ := a.Bus().Subscribe(app.TopicQueriesChanged, 16)
	defer cancelQ()
	a.Start()
	t.Cleanup(a.Stop)
	if a.SDKAddr() == "" {
		t.Fatal("sdk server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws://"+a.SDKAddr()+sdkws.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	hello, _ := protocol.Encode(protocol.TypeHello, "", protocol.Hello{
		SDKVersion: "0.1.0", ProtocolVersion: 1,
		App: protocol.AppInfo{ID: "com.example.app", Platform: "ios"}, Device: protocol.DeviceInfo{ID: "d1"},
		Features: []string{protocol.FeatureQueries},
	})
	if err := c.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil { // welcome
		t.Fatal(err)
	}
	waitTopic(t, queriesCh, "queries:changed after connect")
	if devs := a.Queries().Devices(); len(devs) != 1 || devs[0].App.ID != "com.example.app" {
		t.Fatalf("devices = %+v", devs)
	}

	snapshot, err := os.ReadFile("../../protocol/fixtures/queries.snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageText, snapshot); err != nil {
		t.Fatal(err)
	}
	waitTopic(t, queriesCh, "queries:changed after snapshot")
	list, _, err := a.Queries().Queries(queries.DeviceKey("com.example.app", "d1"))
	if err != nil || len(list) != 1 {
		t.Fatalf("queries = %+v err = %v", list, err)
	}
}

func TestSDKServerFeedsTheStorageFeature(t *testing.T) {
	t.Parallel()
	a := app.New(app.Config{MetroEndpoints: []string{"http://127.0.0.1:1"}, PollInterval: time.Hour, SDKAddr: "127.0.0.1:0"})
	storageCh, cancelS := a.Bus().Subscribe(app.TopicStorageChanged, 16)
	defer cancelS()
	a.Start()
	t.Cleanup(a.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws://"+a.SDKAddr()+sdkws.Path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	hello, _ := protocol.Encode(protocol.TypeHello, "", protocol.Hello{
		SDKVersion: "0.1.0", ProtocolVersion: 1,
		App: protocol.AppInfo{ID: "com.example.app", Platform: "ios"}, Device: protocol.DeviceInfo{ID: "d1"},
		Features: []string{protocol.FeatureStorage},
	})
	if err := c.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Read(ctx); err != nil { // welcome
		t.Fatal(err)
	}
	waitTopic(t, storageCh, "storage:changed after connect")

	snapshot, err := os.ReadFile("../../protocol/fixtures/storage.snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageText, snapshot); err != nil {
		t.Fatal(err)
	}
	waitTopic(t, storageCh, "storage:changed after snapshot")
	key := storage.DeviceKey("com.example.app", "d1")
	stores, err := a.Storage().Stores(key)
	if err != nil || len(stores) != 3 {
		t.Fatalf("stores = %+v err = %v", stores, err)
	}

	// A GET goes out as storage.action and its storage.result answers it.
	got := make(chan error, 1)
	go func() {
		v, err := a.Storage().Get(ctx, key, "async-storage", "theme")
		if err == nil && v.Value != `"dark"` {
			err = fmt.Errorf("value = %+v", v)
		}
		got <- err
	}()
	_, raw, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := protocol.Decode(raw)
	if err != nil || msg.Type != protocol.TypeStorageAction {
		t.Fatalf("frame = %s (%v)", raw, err)
	}
	reply, _ := protocol.Encode(protocol.TypeStorageResult, msg.ID, protocol.StorageResult{
		ID: msg.ID, OK: true, Value: &protocol.StorageValue{Type: protocol.StorageValueString, Value: `"dark"`},
	})
	if err := c.Write(ctx, websocket.MessageText, reply); err != nil {
		t.Fatal(err)
	}
	if err := <-got; err != nil {
		t.Fatalf("Get: %v", err)
	}
	if devs := a.Queries().Devices(); len(devs) != 1 {
		t.Fatalf("queries devices = %+v; connect still reaches the queries feature", devs)
	}
}

func TestDetachReleasesARecordingInProgress(t *testing.T) {
	t.Parallel()
	debugger := cdptest.New(t)
	debugger.Handle("Tracing.start", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	// The app never reports tracingComplete: only a detach can end the wait.
	debugger.Handle("Tracing.end", func(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil })
	m := newFakeMetro(t, debugger)
	a := app.New(app.Config{MetroEndpoints: []string{m.srv.URL}, PollInterval: 10 * time.Millisecond, SDKAddr: "off"})
	targetsCh, cancelTargets := a.Bus().Subscribe(app.TopicTargetsChanged, 8)
	defer cancelTargets()
	a.Start()
	t.Cleanup(a.Stop)
	waitTopic(t, targetsCh, "targets:changed")
	if err := a.Attach(a.Targets()[0].ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for a.SessionSnapshot().State != targets.StateAttached {
		time.Sleep(5 * time.Millisecond)
	}
	ctx := context.Background()
	if err := a.Performance().StartTrace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { _, err := a.Performance().StopTrace(ctx); errc <- err }()
	for !slices.Contains(debugger.Methods(), "Tracing.end") {
		time.Sleep(5 * time.Millisecond)
	}
	a.Detach()
	select {
	case err := <-errc:
		if !errors.Is(err, perf.ErrSessionEnded) {
			t.Fatalf("StopTrace after detach = %v, want ErrSessionEnded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopTrace kept waiting after the detach")
	}
	if a.Performance().Status().Tracing {
		t.Fatal("still tracing after the detach")
	}
}
