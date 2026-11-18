package metro_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Reidevl/rn-lens/internal/adapters/metro"
)

func TestHMRDialerExchangesJSON(t *testing.T) {
	t.Parallel()
	got := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hot" {
			http.NotFound(w, r)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		var reg map[string]any
		if wsjson.Read(r.Context(), c, &reg) != nil {
			return
		}
		got <- reg
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "bundle-registered"})
		_, _, _ = c.Read(r.Context())
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := metro.HMRDialer{}.DialHMR(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/hot")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Send(ctx, map[string]any{"type": "register-entrypoints", "entryPoints": []string{"http://x/index.bundle"}}); err != nil {
		t.Fatal(err)
	}
	if reg := <-got; reg["type"] != "register-entrypoints" {
		t.Fatalf("server read %v", reg)
	}
	msg, err := s.Recv(ctx)
	if err != nil || string(msg) != `{"type":"bundle-registered"}` {
		t.Fatalf("Recv = %s, %v", msg, err)
	}
}
