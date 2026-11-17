package targets

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/platform/clock"
)

// State of a session.
type State string

// Session states.
const (
	StateIdle         State = "idle"         // nothing attached
	StateConnecting   State = "connecting"   // dialing or enabling domains
	StateAttached     State = "attached"     // live connection
	StateReconnecting State = "reconnecting" // connection lost, retrying with backoff
	StateEvicted      State = "evicted"      // another debugger took the slot; user action required
	StateClosed       State = "closed"       // detached by the user
)

// Conn is the slice of a CDP connection the session needs.
// *cdpws.Conn satisfies it.
type Conn interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Events() <-chan cdp.Event
	Done() <-chan struct{}
	Reason() cdp.CloseReason
	Close() error
}

// Dialer opens CDP connections. The composition root provides one backed by cdpws.
type Dialer interface {
	Dial(ctx context.Context, url string) (Conn, error)
}

// DialerFunc adapts a function to the Dialer interface.
type DialerFunc func(ctx context.Context, url string) (Conn, error)

// Dial implements Dialer.
func (f DialerFunc) Dial(ctx context.Context, url string) (Conn, error) { return f(ctx, url) }

// EventHandler receives every CDP event of the attached connection, in order,
// on the session goroutine. Handlers must be quick.
type EventHandler func(ev cdp.Event)

// Snapshot is the observable state of a session.
type Snapshot struct {
	State    State     `json:"state"`
	Target   *Target   `json:"target,omitempty"`
	Metadata *Metadata `json:"metadata,omitempty"`
	// Reason explains reconnecting/evicted/closed states for the UI.
	Reason   string    `json:"reason,omitempty"`
	Attempts int       `json:"attempts"`
	Since    time.Time `json:"since"`
}

// SessionOptions configures a Session.
type SessionOptions struct {
	Clock clock.Clock
	// BackoffMin/BackoffMax bound the reconnect delay. Zero means 500ms / 5s.
	BackoffMin time.Duration
	BackoffMax time.Duration
	// EnableTimeout bounds the ReactNativeApplication.enable call. Zero means 5s.
	EnableTimeout time.Duration
	// OnChange is called after every state change. It must not block.
	OnChange func(Snapshot)
	Logger   *slog.Logger
}

// Session attaches to one target at a time and keeps the connection alive.
type Session struct {
	dialer Dialer
	opts   SessionOptions

	mu       sync.RWMutex
	snapshot Snapshot
	conn     Conn
	cancel   context.CancelFunc
	finished chan struct{}
	handlers []EventHandler
}

// ErrNotAttached is returned by actions that need a live connection.
var ErrNotAttached = errors.New("targets: no attached session")

// NewSession creates an idle session.
func NewSession(dialer Dialer, opts SessionOptions) *Session {
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.BackoffMin <= 0 {
		opts.BackoffMin = 500 * time.Millisecond
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = 5 * time.Second
	}
	if opts.EnableTimeout <= 0 {
		opts.EnableTimeout = 5 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Session{
		dialer:   dialer,
		opts:     opts,
		snapshot: Snapshot{State: StateIdle, Since: opts.Clock.Now()},
	}
}

// OnEvent registers a handler for CDP events of the attached connection.
func (s *Session) OnEvent(h EventHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, h)
}

// Snapshot returns the current observable state.
func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

// Attach starts a session with target. Any previous session is detached first.
// It returns once the background loop is running; watch OnChange or Snapshot
// for progress.
func (s *Session) Attach(target Target) {
	s.Detach()

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})

	s.mu.Lock()
	s.cancel = cancel
	s.finished = finished
	t := target
	s.snapshot = Snapshot{State: StateConnecting, Target: &t, Since: s.opts.Clock.Now()}
	snap := s.snapshot
	s.mu.Unlock()
	s.emit(snap)

	go s.run(ctx, target, finished)
}

// Detach closes the current connection and moves to the closed state.
// It blocks until the background loop has stopped.
func (s *Session) Detach() {
	s.mu.Lock()
	cancel := s.cancel
	finished := s.finished
	s.cancel = nil
	s.finished = nil
	s.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-finished
}

// Call forwards a CDP request to the attached connection.
func (s *Session) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.RLock()
	conn := s.conn
	state := s.snapshot.State
	s.mu.RUnlock()
	if conn == nil || state != StateAttached {
		return nil, ErrNotAttached
	}
	return conn.Call(ctx, method, params)
}

// Reload asks the app to reload its JavaScript bundle.
func (s *Session) Reload(ctx context.Context) error {
	_, err := s.Call(ctx, cdp.MethodPageReload, nil)
	return err
}

func (s *Session) run(ctx context.Context, target Target, finished chan struct{}) {
	defer close(finished)
	defer s.setConn(nil)

	attempts := 0
	for {
		conn, err := s.dialer.Dial(ctx, target.WebSocketURL)
		if err != nil {
			if ctx.Err() != nil {
				s.transition(StateClosed, "detached", 0)
				return
			}
			attempts++
			s.transition(StateReconnecting, "dial failed: "+err.Error(), attempts)
			if !s.wait(ctx, attempts) {
				s.transition(StateClosed, "detached", 0)
				return
			}
			continue
		}

		s.setConn(conn)
		s.enableApplicationDomain(ctx, conn)
		if ctx.Err() != nil {
			_ = conn.Close()
			s.transition(StateClosed, "detached", 0)
			return
		}
		attempts = 0
		s.transition(StateAttached, "", 0)

		reason := s.pump(ctx, conn)
		s.setConn(nil)

		switch {
		case ctx.Err() != nil:
			_ = conn.Close()
			s.transition(StateClosed, "detached", 0)
			return
		case reason.Code == cdp.CloseNewDebuggerOpened:
			s.transition(StateEvicted, reason.String(), 0)
			return
		default:
			attempts++
			s.transition(StateReconnecting, reason.String(), attempts)
			if !s.wait(ctx, attempts) {
				s.transition(StateClosed, "detached", 0)
				return
			}
		}
	}
}

// enableApplicationDomain performs the Fusebox handshake: the frontend must
// enable ReactNativeApplication before any other domain so the app treats it
// as a first-class DevTools client and sends metadataUpdated.
func (s *Session) enableApplicationDomain(ctx context.Context, conn Conn) {
	callCtx, cancel := context.WithTimeout(ctx, s.opts.EnableTimeout)
	defer cancel()
	if _, err := conn.Call(callCtx, cdp.MethodReactNativeApplicationEnable, nil); err != nil {
		// Pre-0.76 targets do not know the domain. Lens still works, with less metadata.
		s.opts.Logger.Debug("ReactNativeApplication.enable failed", "err", err)
	}
}

// pump forwards events until the connection ends and returns the close reason.
func (s *Session) pump(ctx context.Context, conn Conn) cdp.CloseReason {
	for {
		select {
		case <-ctx.Done():
			return cdp.CloseReason{Message: "detached"}
		case ev, ok := <-conn.Events():
			if !ok {
				return conn.Reason()
			}
			s.handleEvent(ev)
		case <-conn.Done():
			// Drain anything already queued so handlers see every event.
			for ev := range conn.Events() {
				s.handleEvent(ev)
			}
			return conn.Reason()
		}
	}
}

func (s *Session) handleEvent(ev cdp.Event) {
	if ev.Method == cdp.EventReactNativeMetadataUpdated {
		var md Metadata
		if err := json.Unmarshal(ev.Params, &md); err == nil {
			s.setMetadata(md)
		}
	}
	s.mu.RLock()
	handlers := s.handlers
	s.mu.RUnlock()
	for _, h := range handlers {
		h(ev)
	}
}

func (s *Session) wait(ctx context.Context, attempt int) bool {
	delay := s.opts.BackoffMin
	for i := 1; i < attempt && delay < s.opts.BackoffMax; i++ {
		delay *= 2
	}
	delay = min(delay, s.opts.BackoffMax)
	select {
	case <-ctx.Done():
		return false
	case <-s.opts.Clock.After(delay):
		return true
	}
}

func (s *Session) setConn(conn Conn) {
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
}

func (s *Session) setMetadata(md Metadata) {
	s.mu.Lock()
	s.snapshot.Metadata = &md
	snap := s.snapshot
	s.mu.Unlock()
	s.emit(snap)
}

func (s *Session) transition(state State, reason string, attempts int) {
	s.mu.Lock()
	s.snapshot.State = state
	s.snapshot.Reason = reason
	s.snapshot.Attempts = attempts
	s.snapshot.Since = s.opts.Clock.Now()
	if state == StateClosed || state == StateEvicted {
		s.snapshot.Metadata = nil
	}
	snap := s.snapshot
	s.mu.Unlock()
	s.emit(snap)
}

func (s *Session) emit(snap Snapshot) {
	if s.opts.OnChange != nil {
		s.opts.OnChange(snap)
	}
}
