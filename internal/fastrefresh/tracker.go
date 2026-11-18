// Package fastrefresh follows Metro's hot module reloading for the attached
// app: Lens registers as an HMR client on Metro's /hot socket with the app's
// bundle URL and reports each Fast Refresh (updating, updated, or the
// bundling error).
package fastrefresh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

// Phase is where the last Fast Refresh stands.
type Phase string

// Phases reported by State.
const (
	PhaseOff         Phase = "off"         // no app attached
	PhaseUnavailable Phase = "unavailable" // the app does not load its bundle from Metro
	PhaseConnecting  Phase = "connecting"  // (re)connecting to Metro's /hot socket
	PhaseReady       Phase = "ready"       // registered, no Fast Refresh yet
	PhaseUpdating    Phase = "updating"    // Metro is rebuilding after a file change
	PhaseUpdated     Phase = "updated"     // the app received the last update
	PhaseError       Phase = "error"       // the last rebuild failed
)

// State is the Fast Refresh status shown in the status bar.
type State struct {
	Phase Phase `json:"phase"`
	// Modules counts the modules added, changed or deleted by the last update.
	Modules int `json:"modules,omitempty"`
	// At is when the last update or error happened.
	At time.Time `json:"at,omitzero"`
	// Message explains PhaseError and PhaseUnavailable.
	Message string `json:"message,omitempty"`
}

// Stream is one connection to Metro's /hot WebSocket.
type Stream interface {
	Send(ctx context.Context, v any) error
	Recv(ctx context.Context) ([]byte, error)
	Close() error
}

// Dialer opens Metro's /hot WebSocket.
type Dialer interface {
	DialHMR(ctx context.Context, url string) (Stream, error)
}

// Caller sends CDP commands to the attached app.
type Caller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// Options configures a Tracker.
type Options struct {
	// OnChange is called after State changes. It must not block.
	OnChange   func()
	Logger     *slog.Logger
	BackoffMin time.Duration // zero means 500 ms
	BackoffMax time.Duration // zero means 10 s
	Now        func() time.Time
}

// Tracker keeps one HMR registration for the attached app.
type Tracker struct {
	dialer Dialer
	caller Caller
	opts   Options

	mu     sync.Mutex
	state  State
	gen    uint64 // the run allowed to change state
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New returns a Tracker in PhaseOff.
func New(d Dialer, c Caller, opts Options) *Tracker {
	if opts.OnChange == nil {
		opts.OnChange = func() {}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.BackoffMin <= 0 {
		opts.BackoffMin = 500 * time.Millisecond
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = 10 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Tracker{dialer: d, caller: c, opts: opts, state: State{Phase: PhaseOff}}
}

// State returns the current Fast Refresh status.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Attached starts following the app attached through the Metro at endpoint
// (its base URL), replacing any previous registration. It does not wait for
// the previous registration to end, so it is safe on the session goroutine.
func (t *Tracker) Attached(endpoint string) {
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.gen++
	gen := t.gen
	t.cancel = cancel
	t.mu.Unlock()
	t.set(gen, State{Phase: PhaseConnecting})
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.run(ctx, gen, endpoint)
	}()
}

// Detached ends the registration and reports PhaseOff.
func (t *Tracker) Detached() {
	t.mu.Lock()
	gen := t.endLocked()
	t.mu.Unlock()
	t.set(gen, State{Phase: PhaseOff})
}

// Stop ends the registration and waits for it; call it on shutdown.
func (t *Tracker) Stop() {
	t.mu.Lock()
	t.endLocked()
	t.mu.Unlock()
	t.wg.Wait()
}

func (t *Tracker) endLocked() uint64 {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.gen++
	return t.gen
}

// set changes the state when gen is still the current run.
func (t *Tracker) set(gen uint64, s State) {
	t.mu.Lock()
	changed := t.gen == gen && t.state != s
	if changed {
		t.state = s
	}
	t.mu.Unlock()
	if changed {
		t.opts.OnChange()
	}
}

func (t *Tracker) run(ctx context.Context, gen uint64, endpoint string) {
	bundle, err := t.bundleURL(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		t.set(gen, State{Phase: PhaseUnavailable, Message: err.Error()})
		return
	}
	hot, err := hotURL(endpoint)
	if err != nil {
		t.set(gen, State{Phase: PhaseUnavailable, Message: err.Error()})
		return
	}
	backoff := t.opts.BackoffMin
	for {
		if t.session(ctx, gen, hot, bundle) {
			backoff = t.opts.BackoffMin
		}
		if ctx.Err() != nil {
			return
		}
		t.set(gen, State{Phase: PhaseConnecting})
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, t.opts.BackoffMax)
	}
}

// session registers once and follows the socket until it fails. It reports
// whether the registration succeeded.
func (t *Tracker) session(ctx context.Context, gen uint64, hot, bundle string) bool {
	s, err := t.dialer.DialHMR(ctx, hot)
	if err != nil {
		t.opts.Logger.Debug("fast refresh: dial failed", "url", hot, "err", err)
		return false
	}
	defer func() { _ = s.Close() }()
	if err := s.Send(ctx, map[string]any{"type": "register-entrypoints", "entryPoints": []string{bundle}}); err != nil {
		return false
	}
	t.set(gen, State{Phase: PhaseReady})

	initial, modules := false, 0
	for {
		raw, err := s.Recv(ctx)
		if err != nil {
			return true
		}
		var msg struct {
			Type string `json:"type"`
			Body struct {
				IsInitialUpdate bool              `json:"isInitialUpdate"`
				Added           []json.RawMessage `json:"added"`
				Modified        []json.RawMessage `json:"modified"`
				Deleted         []json.RawMessage `json:"deleted"`
				Message         string            `json:"message"`
			} `json:"body"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "update-start":
			initial, modules = msg.Body.IsInitialUpdate, 0
			if !initial {
				t.set(gen, State{Phase: PhaseUpdating})
			}
		case "update":
			modules = len(msg.Body.Added) + len(msg.Body.Modified) + len(msg.Body.Deleted)
		case "update-done":
			if !initial {
				t.set(gen, State{Phase: PhaseUpdated, Modules: modules, At: t.opts.Now()})
			}
			initial = false
		case "error":
			message := msg.Body.Message
			if message == "" {
				message = "Metro could not rebuild the bundle."
			}
			t.set(gen, State{Phase: PhaseError, Message: message, At: t.opts.Now()})
		}
	}
}

// bundleExpression reads the URL the app loaded its JavaScript from.
const bundleExpression = `(() => {
  try {
    const sc = globalThis.nativeModuleProxy && globalThis.nativeModuleProxy.SourceCode;
    const url = sc && sc.getConstants ? sc.getConstants().scriptURL : sc && sc.scriptURL;
    return typeof url === "string" ? url : null;
  } catch (e) {
    return null;
  }
})()`

var errNoBundle = errors.New("the app does not load its bundle from Metro")

func (t *Tracker) bundleURL(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		raw, err := t.caller.Call(callCtx, cdp.MethodRuntimeEvaluate, cdp.EvaluateParams{Expression: bundleExpression, ReturnByValue: true})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		var res cdp.EvaluateResult
		if err := json.Unmarshal(raw, &res); err != nil {
			return "", err
		}
		var s string
		if json.Unmarshal(res.Result.Value, &s) != nil || s == "" {
			return "", errNoBundle
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", errNoBundle
		}
		return s, nil
	}
	return "", fmt.Errorf("could not read the bundle URL: %w", lastErr)
}

// hotURL turns a Metro base URL into its /hot WebSocket URL.
func hotURL(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid Metro address %q", endpoint)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path, u.RawQuery, u.Fragment = "/hot", "", ""
	return u.String(), nil
}
