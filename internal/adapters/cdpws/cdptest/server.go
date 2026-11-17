// Package cdptest provides an in-process fake of Metro's inspector proxy
// debugger endpoint for tests. It speaks plain CDP frames over WebSocket, the
// same thing a real /inspector/debug socket delivers after unwrapping.
package cdptest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Request is a decoded client request handed to a Handler.
type Request struct {
	ID     int64
	Method string
	Params json.RawMessage
}

// Handler produces the result (or a protocol error) for a request.
// Returning a nil result and nil error sends an empty object.
type Handler func(req Request) (result any, err *Error)

// Error is a protocol error a Handler can return.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server is a fake debugger endpoint. Create it with New.
type Server struct {
	t         testing.TB
	http      *httptest.Server
	ctx       context.Context
	cancel    context.CancelFunc
	serving   sync.WaitGroup
	mu        sync.Mutex
	handlers  map[string]Handler
	conns     []*websocket.Conn
	requests  []Request
	accepted  int
	connected chan struct{}
}

// New starts a server. When the test ends every accepted socket is closed
// and the serve loops are waited for, so a test that forgets to close its
// client cannot leak a goroutine blocked in Read.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{
		t:         tb,
		handlers:  make(map[string]Handler),
		connected: make(chan struct{}, 16),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.http = httptest.NewServer(http.HandlerFunc(s.serve))
	tb.Cleanup(func() {
		s.cancel()
		s.http.Close()
		idle := make(chan struct{})
		go func() {
			s.serving.Wait()
			close(idle)
		}()
		select {
		case <-idle:
		case <-time.After(5 * time.Second):
			tb.Errorf("cdptest: serve loop still running after cleanup")
		}
	})
	return s
}

// URL returns the ws:// URL clients should dial.
func (s *Server) URL() string {
	return "ws" + strings.TrimPrefix(s.http.URL, "http") + "/inspector/debug?device=1&page=1"
}

// Handle registers the handler for method. Unhandled methods get a
// "method not found" error (-32601), as Hermes does.
func (s *Server) Handle(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Requests returns every request received so far, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	copy(out, s.requests)
	return out
}

// Methods returns the method names of every request received, in order.
func (s *Server) Methods() []string {
	reqs := s.Requests()
	out := make([]string, len(reqs))
	for i, r := range reqs {
		out[i] = r.Method
	}
	return out
}

// ConnectionCount returns how many WebSocket connections were accepted in total.
func (s *Server) ConnectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

// WaitConnected blocks until a client connects or the timeout expires.
func (s *Server) WaitConnected(timeout time.Duration) bool {
	select {
	case <-s.connected:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Emit pushes an event to every connected client.
func (s *Server) Emit(method string, params any) {
	raw, err := json.Marshal(map[string]any{"method": method, "params": params})
	if err != nil {
		s.t.Fatalf("cdptest: marshal event: %v", err)
	}
	s.EmitRaw(raw)
}

// EmitRaw writes raw as one text frame to every connected client, without
// validating it. Use it to send malformed or hand-built frames.
func (s *Server) EmitRaw(raw []byte) {
	for _, c := range s.clients() {
		_ = c.Write(s.ctx, websocket.MessageText, raw)
	}
}

// CloseAll closes every client connection with the given status and reason,
// mimicking the proxy's "[CODE] message" convention.
func (s *Server) CloseAll(status websocket.StatusCode, reason string) {
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Close(status, reason)
	}
}

// DropAll ends every client connection without a close frame, the way a
// crashed proxy or a reset tunnel does. Clients see a transport error.
func (s *Server) DropAll() {
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.CloseNow()
	}
}

func (s *Server) clients() []*websocket.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*websocket.Conn(nil), s.conns...)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.serving.Add(1)
	defer s.serving.Done()

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	s.mu.Lock()
	s.conns = append(s.conns, c)
	s.accepted++
	s.mu.Unlock()
	select {
	case s.connected <- struct{}{}:
	default:
	}

	// Reading with the server context closes the socket when the test ends.
	ctx := s.ctx
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			return
		}
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}
		request := Request{ID: req.ID, Method: req.Method, Params: req.Params}

		s.mu.Lock()
		s.requests = append(s.requests, request)
		h := s.handlers[req.Method]
		s.mu.Unlock()

		reply := map[string]any{"id": req.ID}
		if h == nil {
			reply["error"] = Error{Code: -32601, Message: "'" + req.Method + "' wasn't found"}
		} else if result, perr := h(request); perr != nil {
			reply["error"] = perr
		} else if result == nil {
			reply["result"] = map[string]any{}
		} else {
			reply["result"] = result
		}
		out, _ := json.Marshal(reply)
		if err := c.Write(ctx, websocket.MessageText, out); err != nil {
			return
		}
	}
}
