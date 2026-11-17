package cdptest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
)

// TestCleanupClosesClientsThatWereNeverClosed proves the fake does not leak
// a serve loop when a test forgets to close its client: once the subtest's
// cleanup ran, the raw socket is closed from the server side.
func TestCleanupClosesClientsThatWereNeverClosed(t *testing.T) {
	// Not parallel: the outer test must observe the subtest's cleanup first.
	var ws *websocket.Conn
	t.Run("leaky client", func(t *testing.T) {
		srv := cdptest.New(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		ws, _, err = websocket.Dial(ctx, srv.URL(), nil)
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		if !srv.WaitConnected(time.Second) {
			t.Fatal("client did not connect")
		}
	})
	t.Cleanup(func() { _ = ws.CloseNow() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := ws.Read(ctx)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Read after server cleanup = %v, want the socket closed by the server", err)
	}
}
