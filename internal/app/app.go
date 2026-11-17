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
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/platform/bus"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// Notification topics published on the bus. Payloads are empty: subscribers
// pull fresh state (ADR-0003).
const (
	TopicTargetsChanged = "targets:changed"
	TopicSessionChanged = "session:changed"
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
	Logger        *slog.Logger
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

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	stopped   chan struct{}
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
		OnChange: func(targets.Snapshot) { a.bus.Publish(TopicSessionChanged, nil) },
	})
	return a
}

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

// Stop detaches any session and stops discovery. It is idempotent.
func (a *App) Stop() {
	a.stopOnce.Do(func() {
		a.session.Detach()
		if a.cancel != nil {
			a.cancel()
			<-a.stopped
		}
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
		out = append(out, targets.FromMetro(d))
	}
	return out
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

func (a *App) dial(ctx context.Context, url string) (targets.Conn, error) {
	header := http.Header{"User-Agent": []string{"rnlens/" + a.cfg.Version}}
	return cdpws.Dial(ctx, url, cdpws.Options{
		HTTPClient: a.cfg.HTTPClient,
		Header:     header,
		OnFrame:    a.cfg.FrameObserver,
	})
}
