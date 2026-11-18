// Package sdkws is the WebSocket server @rnlens/sdk connects to. It owns the
// handshake (hello/welcome, protocol version, optional pairing token), keeps
// one session per app+device, pings for liveness and hands decoded frames to
// a feature handler.
package sdkws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// Close codes Lens sends; the SDK shows them in its diagnostics.
const (
	CloseHelloTimeout   websocket.StatusCode = 4001
	CloseBadVersion     websocket.StatusCode = 4002
	CloseBadToken       websocket.StatusCode = 4003
	CloseReplaced       websocket.StatusCode = 4004
	ClosePingTimeout    websocket.StatusCode = 4005
	CloseProtocolError  websocket.StatusCode = 4006
	CloseServerShutdown websocket.StatusCode = 1001
	defaultHelloTimeout                      = 5 * time.Second
	defaultPingInterval                      = 15 * time.Second
	defaultMissedPings                       = 2
	defaultWriteTimeout                      = 5 * time.Second
	maxFrameBytes                            = 8 << 20
)

// Path is the WebSocket path the SDK dials.
const Path = "/lens"

// SessionKey identifies an app instance on a device.
type SessionKey struct {
	AppID    string
	DeviceID string
}

// Handler receives connection events. Calls for one session arrive in order
// on that session's goroutine; different sessions may call concurrently.
type Handler interface {
	OnConnect(s *Session)
	OnMessage(s *Session, msg protocol.Message)
	OnDisconnect(s *Session, reason error)
}

// Options configures a Server.
type Options struct {
	// Addr to listen on. Empty means ":7420"; ":0" picks a free port (tests).
	Addr string
	// Token, when set, must match hello.payload.token.
	Token string
	// LensVersion is reported in welcome.
	LensVersion  string
	HelloTimeout time.Duration
	PingInterval time.Duration
	MissedPings  int
	Logger       *slog.Logger
}

// Server accepts SDK connections.
type Server struct {
	opts    Options
	handler Handler

	mu       sync.RWMutex
	sessions map[SessionKey]*Session
	// conns holds every accepted socket, registered or still in its
	// handshake, so Stop can cut stragglers once its context expires.
	conns    map[*websocket.Conn]struct{}
	nextSeq  uint64
	http     *http.Server
	listener net.Listener
	wg       sync.WaitGroup
	closed   bool
}

// New creates a server that reports to handler.
func New(opts Options, handler Handler) *Server {
	if opts.Addr == "" {
		opts.Addr = ":7420"
	}
	if opts.HelloTimeout <= 0 {
		opts.HelloTimeout = defaultHelloTimeout
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = defaultPingInterval
	}
	if opts.MissedPings <= 0 {
		opts.MissedPings = defaultMissedPings
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.LensVersion == "" {
		opts.LensVersion = "dev"
	}
	return &Server{
		opts: opts, handler: handler,
		sessions: make(map[SessionKey]*Session),
		conns:    make(map[*websocket.Conn]struct{}),
	}
}

// Start listens and serves in the background. It returns the bound address.
func (s *Server) Start() (string, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", s.opts.Addr)
	if err != nil {
		return "", fmt.Errorf("sdkws: listen %s: %w", s.opts.Addr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(Path, s.serveWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	s.listener = ln
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.opts.Logger.Error("sdk server stopped", "err", err)
		}
	}()
	s.opts.Logger.Info("sdk server listening", "addr", ln.Addr().String(), "path", Path)
	return ln.Addr().String(), nil
}

// Addr returns the bound address, or "" before Start.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop closes every session and the listener, refuses connections that
// arrive from now on and returns once every session goroutine has delivered
// its OnDisconnect, or once ctx expires. On expiry it cuts the sockets still
// open (a peer that has not answered its close frame, a connection still
// waiting for its hello) without waiting for their goroutines, and returns
// ctx.Err().
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		// Close concurrently: each close handshake waits for its peer (up to
		// the library's 5 s budget while the session's Read holds the socket),
		// so a sequential loop would stall every later session behind a slow
		// one.
		var closes sync.WaitGroup
		for _, sess := range sessions {
			closes.Add(1)
			go func(sess *Session) {
				defer closes.Done()
				sess.Close(CloseServerShutdown, "lens is shutting down")
			}(sess)
		}
		closes.Wait()
		// Shutdown closes the listener; it does not track hijacked sockets,
		// which is why the wait group counts connections too.
		s.wg.Wait()
	}()
	var err error
	if s.http != nil {
		err = s.http.Shutdown(ctx)
	}
	select {
	case <-settled:
		return err
	case <-ctx.Done():
	}
	// Cut the stragglers but do not wait for them: CloseNow on a socket whose
	// close handshake is in flight blocks until that handshake gives up, and
	// a handler may still be inside OnDisconnect.
	s.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.mu.Unlock()
	for _, conn := range conns {
		go func(conn *websocket.Conn) { _ = conn.CloseNow() }(conn)
	}
	if err == nil {
		err = ctx.Err()
	}
	return err
}

// track records conn as a live socket and adds the calling connection
// goroutine to the wait group, unless Stop already began. Adding under the
// mutex that Stop takes before it waits keeps every Add ordered before the
// Wait; the sequence taken under the same lock orders Sessions.
func (s *Server) track(conn *websocket.Conn, remote string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false
	}
	s.wg.Add(1)
	s.nextSeq++
	s.conns[conn] = struct{}{}
	return &Session{server: s, conn: conn, remote: remote, connectedAt: time.Now(), seq: s.nextSeq}, true
}

// untrack forgets conn and releases its wait-group slot.
func (s *Server) untrack(conn *websocket.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	s.wg.Done()
}

// Sessions lists live sessions, most recently accepted last. The order is
// by acceptance sequence, which never ties, so it is deterministic even
// when two sockets were accepted at the same instant.
func (s *Server) Sessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq < out[j].seq })
	return out
}

// Session returns the live session for key.
func (s *Server) Session(key SessionKey) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[key]
	return sess, ok
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	// React Native's WebSocket sends no Origin and physical devices come from
	// the LAN, so origin checks cannot protect this endpoint; the optional
	// token does (SECURITY.md).
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	conn.SetReadLimit(maxFrameBytes)
	sess, ok := s.track(conn, r.RemoteAddr)
	if !ok {
		_ = conn.Close(CloseServerShutdown, "lens is shutting down")
		return
	}
	defer s.untrack(conn)

	if err := sess.handshake(); err != nil {
		s.opts.Logger.Info("sdk handshake rejected", "remote", r.RemoteAddr, "err", err)
		return
	}
	if !s.register(sess) {
		// Stop began during the handshake and did not see this session.
		sess.Close(CloseServerShutdown, "lens is shutting down")
		return
	}
	sess.run()
}

// register makes sess the live session for its key, replacing a previous
// one. It reports false once Stop began.
func (s *Server) register(sess *Session) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	old, had := s.sessions[sess.Key]
	s.sessions[sess.Key] = sess
	s.mu.Unlock()
	if had {
		// The close handshake waits for the peer; never block the new session on it.
		go old.Close(CloseReplaced, "another connection for the same app and device replaced this one")
	}
	return true
}

func (s *Server) unregister(sess *Session) {
	s.mu.Lock()
	if cur, ok := s.sessions[sess.Key]; ok && cur == sess {
		delete(s.sessions, sess.Key)
	}
	s.mu.Unlock()
}

func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
