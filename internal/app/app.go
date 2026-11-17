// Package app is the composition root of Lens.
//
// It wires feature services together without importing any UI framework, so
// the same core runs under the Wails desktop shell, a headless CLI, or tests.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/platform/bus"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// Notification topics published on the bus. Payloads are empty: subscribers
// pull fresh state (ADR-0003).
const (
	TopicTargetsChanged = "targets:changed"
	TopicSessionChanged = "session:changed"
	TopicConsoleChanged = "console:changed"
)

// Config holds the values the composition root needs from its caller.
type Config struct {
	// Version is the build version shown in the UI ("dev" for local builds).
	Version string
	// MetroEndpoints are the Metro base URLs to poll. Empty means metro.DefaultEndpoints.
	MetroEndpoints []string
	// PollInterval overrides the discovery interval (tests). Zero means one second.
	PollInterval time.Duration
	// HTTPClient is used for Metro requests and WebSocket handshakes. Nil means a 3 s timeout client.
	HTTPClient *http.Client
	// FrameObserver, when set, sees every CDP frame (used by `rnlens record`).
	FrameObserver func(dir cdp.Direction, raw []byte)
	// ConsoleCapacity bounds the console log. Zero means console.DefaultCapacity.
	ConsoleCapacity int
	Logger          *slog.Logger
}

// Info describes the running Lens instance.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ErrUnknownTarget is returned when an id does not match a discovered target.
var ErrUnknownTarget = errors.New("app: unknown target")

// App owns every feature service for the lifetime of the process.
type App struct {
	cfg       Config
	bus       *bus.Bus
	metro     *metro.Client
	discovery *metro.Discovery
	session   *targets.Session
	console   *console.Service

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	stopped   chan struct{}

	// lastState lets onSessionChange detect transitions: the session also
	// re-emits snapshots (metadata updates) whose state did not change.
	stateMu   sync.Mutex
	lastState targets.State
	// attachMu serializes console domain enabling across fast re-attaches.
	attachMu sync.Mutex
}

// New builds the application graph from cfg. It performs no I/O until Start.
func New(cfg Config) *App {
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}

	a := &App{cfg: cfg, bus: bus.New(), stopped: make(chan struct{})}
	a.metro = metro.NewClient(cfg.HTTPClient)
	a.discovery = metro.NewDiscovery(a.metro, metro.DiscoveryOptions{
		Endpoints: cfg.MetroEndpoints,
		Interval:  cfg.PollInterval,
		Logger:    cfg.Logger,
		OnChange:  func([]metro.Change) { a.bus.Publish(TopicTargetsChanged, nil) },
	})
	a.session = targets.NewSession(targets.DialerFunc(a.dial), targets.SessionOptions{
		Logger:   cfg.Logger,
		OnChange: a.onSessionChange,
		Resolve:  a.resolveTarget,
	})
	a.console = console.NewService(console.NewStore(cfg.ConsoleCapacity, nil), a.session, console.ServiceOptions{
		Notify: func() { a.bus.Publish(TopicConsoleChanged, nil) },
		Logger: cfg.Logger,
	})
	a.session.OnEvent(a.console.HandleEvent)
	return a
}

// onSessionChange fans session transitions out: the UI learns through the
// bus, and features that need the live connection start their domains.
func (a *App) onSessionChange(snap targets.Snapshot) {
	a.bus.Publish(TopicSessionChanged, nil)

	a.stateMu.Lock()
	entered := snap.State == targets.StateAttached && a.lastState != targets.StateAttached
	a.lastState = snap.State
	a.stateMu.Unlock()
	if !entered {
		return
	}
	name := ""
	if snap.Target != nil {
		name = snap.Target.Title
	}
	// Enable asynchronously: the callback runs on the session goroutine and
	// must not wait on the connection it is reporting about.
	go func() {
		a.attachMu.Lock()
		defer a.attachMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.console.OnAttached(ctx, name); err != nil {
			a.cfg.Logger.Warn("console domains not enabled", "err", err)
		}
	}()
}

// Console exposes the console feature to UI adapters.
func (a *App) Console() *console.Service { return a.console }

// Info returns static information about this Lens build.
func (a *App) Info() Info {
	return Info{Name: "Lens", Version: a.cfg.Version}
}

// Bus exposes change notifications to UI adapters.
func (a *App) Bus() *bus.Bus { return a.bus }

// Session exposes the attach session to feature adapters.
func (a *App) Session() *targets.Session { return a.session }

// Start begins Metro discovery in the background. It is idempotent.
func (a *App) Start() {
	a.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		go func() {
			defer close(a.stopped)
			a.discovery.Run(ctx)
		}()
	})
}

// Stop ends discovery first, so nothing can re-attach, then detaches the
// session. It is idempotent.
func (a *App) Stop() {
	a.stopOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
			<-a.stopped
		}
		a.session.Detach()
	})
}

// RefreshTargets polls every Metro endpoint once, synchronously.
func (a *App) RefreshTargets(ctx context.Context) {
	a.discovery.Refresh(ctx)
}

// Targets returns the discovered targets, sorted by endpoint and page id.
func (a *App) Targets() []targets.Target {
	found := a.discovery.Snapshot()
	out := make([]targets.Target, 0, len(found))
	for _, d := range found {
		out = append(out, targetFromMetro(d))
	}
	return out
}

// targetFromMetro converts a discovered Metro page into a targets.Target.
// It lives in the composition root because targets must not depend on the
// Metro adapter (see CONTRIBUTING.md).
func targetFromMetro(d metro.Discovered) targets.Target {
	caps := d.Page.ReactNative.Capabilities
	return targets.Target{
		ID:              d.Endpoint + "|" + d.Page.ID,
		Endpoint:        d.Endpoint,
		MetroID:         d.Page.ID,
		PageID:          d.Page.PageID(),
		Title:           d.Page.Title,
		AppID:           d.Page.AppID,
		DeviceName:      d.Page.DeviceName,
		LogicalDeviceID: d.Page.ReactNative.LogicalDeviceID,
		WebSocketURL:    d.Page.WebSocketDebuggerURL,
		VM:              d.Page.VM,
		Capabilities: targets.Capabilities{
			NativePageReloads:         caps.NativePageReloads,
			NativeSourceCodeFetching:  caps.NativeSourceCodeFetching,
			SupportsMultipleDebuggers: caps.SupportsMultipleDebuggers,
		},
		Reloadable: d.Page.IsReloadable(),
	}
}

// Target looks a discovered target up by id.
func (a *App) Target(id string) (targets.Target, bool) {
	for _, t := range a.Targets() {
		if t.ID == id {
			return t, true
		}
	}
	return targets.Target{}, false
}

// Attach starts a session with the target identified by id.
func (a *App) Attach(id string) error {
	t, ok := a.Target(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownTarget, id)
	}
	a.session.Attach(t)
	return nil
}

// Detach ends the current session.
func (a *App) Detach() { a.session.Detach() }

// SessionSnapshot returns the observable state of the current session.
func (a *App) SessionSnapshot() targets.Snapshot { return a.session.Snapshot() }

// Reload asks the attached app to reload its JavaScript.
func (a *App) Reload(ctx context.Context) error { return a.session.Reload(ctx) }

// OpenInReactNativeDevTools asks Metro to launch React Native DevTools for
// the target. On targets without multiple-debugger support this evicts Lens.
func (a *App) OpenInReactNativeDevTools(ctx context.Context, id string) error {
	t, ok := a.Target(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownTarget, id)
	}
	return a.metro.OpenDebugger(ctx, t.Endpoint, t.MetroID)
}

// resolveTarget is consulted by the session before each reconnect attempt.
// After an app restart Metro can list the app under a new page id, so the
// session follows the app instead of retrying a URL that only answers
// [PAGE_NOT_FOUND]. While the original page is still listed it is kept.
func (a *App) resolveTarget(current targets.Target) (targets.Target, bool) {
	list := a.Targets()
	for _, t := range list {
		if t.ID == current.ID {
			return current, false
		}
	}
	return targets.FindReplacement(current, list)
}

func (a *App) dial(ctx context.Context, wsURL string) (targets.Conn, error) {
	header := http.Header{
		"User-Agent": []string{"rnlens/" + a.cfg.Version},
		// Metro rejects inspector upgrades (401) unless Origin matches its own host.
		"Origin": []string{OriginFor(wsURL)},
	}
	return cdpws.Dial(ctx, wsURL, cdpws.Options{
		HTTPClient: a.cfg.HTTPClient,
		Header:     header,
		OnFrame:    a.cfg.FrameObserver,
	})
}

// OriginFor derives the Origin header Metro expects from a debugger WebSocket
// URL. The inspector proxy only keeps connections whose Origin equals its own
// base URL, which dev servers register as http://127.0.0.1:<port>, so loopback
// hosts (localhost, ::1) are normalised to 127.0.0.1. Other hosts map to
// http(s)://host:port.
func OriginFor(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := "http"
	if strings.EqualFold(u.Scheme, "wss") || strings.EqualFold(u.Scheme, "https") {
		scheme = "https"
	}
	host := u.Host
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "::1", "127.0.0.1", "0.0.0.0":
		host = "127.0.0.1"
		if port := u.Port(); port != "" {
			host += ":" + port
		}
	}
	return scheme + "://" + host
}
