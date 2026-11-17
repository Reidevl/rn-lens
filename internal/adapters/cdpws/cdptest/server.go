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
	mu        sync.Mutex
	handlers  map[string]Handler
	conns     []*websocket.Conn
	requests  []Request
	connected chan struct{}
}

// New starts a server. It is closed automatically when the test ends.
func New(tb testing.TB) *Server {
	tb.Helper()
	s := &Server{
		t:         tb,
		handlers:  make(map[string]Handler),
		connected: make(chan struct{}, 16),
	}
	s.http = httptest.NewServer(http.HandlerFunc(s.serve))
	tb.Cleanup(s.http.Close)
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
	s.mu.Lock()
	conns := append([]*websocket.Conn(nil), s.conns...)
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Write(context.Background(), websocket.MessageText, raw)
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

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	s.mu.Lock()
	s.conns = append(s.conns, c)
	s.mu.Unlock()
	select {
	case s.connected <- struct{}{}:
	default:
	}

	ctx := context.Background()
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
