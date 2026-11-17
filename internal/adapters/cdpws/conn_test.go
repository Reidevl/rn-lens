package cdpws_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

func dial(t *testing.T, srv *cdptest.Server, opts cdpws.Options) *cdpws.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cdpws.Dial(ctx, srv.URL(), opts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestCallReturnsResult(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle("Runtime.evaluate", func(req cdptest.Request) (any, *cdptest.Error) {
		var params struct {
			Expression string `json:"expression"`
		}
		_ = json.Unmarshal(req.Params, &params)
		return map[string]any{"result": map[string]any{"type": "number", "value": 2, "expr": params.Expression}}, nil
	})
	conn := dial(t, srv, cdpws.Options{})

	raw, err := conn.Call(context.Background(), "Runtime.evaluate", map[string]string{"expression": "1+1"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var res struct {
		Result struct {
			Value int    `json:"value"`
			Expr  string `json:"expr"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Result.Value != 2 || res.Result.Expr != "1+1" {
		t.Fatalf("got %+v, want value 2 for 1+1", res.Result)
	}
}

func TestCallSurfacesProtocolErrors(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})

	_, err := conn.Call(context.Background(), "Nope.method", nil)

	var cdpErr *cdp.Error
	if !errors.As(err, &cdpErr) {
		t.Fatalf("error = %v, want *cdp.Error", err)
	}
	if cdpErr.Code != -32601 {
		t.Fatalf("Code = %d, want -32601", cdpErr.Code)
	}
}

func TestConcurrentCallsAreCorrelatedById(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle("Echo", func(req cdptest.Request) (any, *cdptest.Error) {
		var p struct {
			N int `json:"n"`
		}
		_ = json.Unmarshal(req.Params, &p)
		return map[string]int{"n": p.N}, nil
	})
	conn := dial(t, srv, cdpws.Options{})

	const calls = 50
	var wg sync.WaitGroup
	errs := make(chan error, calls)
	for i := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := conn.Call(context.Background(), "Echo", map[string]int{"n": i})
			if err != nil {
				errs <- err
				return
			}
			var res struct {
				N int `json:"n"`
			}
			if err := json.Unmarshal(raw, &res); err != nil || res.N != i {
				errs <- errors.New("mismatched reply")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent call failed: %v", err)
	}
}

func TestEventsAreDeliveredInOrder(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})
	if !srv.WaitConnected(time.Second) {
		t.Fatal("client did not connect")
	}

	srv.Emit("Runtime.consoleAPICalled", map[string]any{"type": "log", "n": 1})
	srv.Emit("Runtime.consoleAPICalled", map[string]any{"type": "log", "n": 2})

	for want := 1; want <= 2; want++ {
		select {
		case ev := <-conn.Events():
			var p struct {
				N int `json:"n"`
			}
			_ = json.Unmarshal(ev.Params, &p)
			if ev.Method != "Runtime.consoleAPICalled" || p.N != want {
				t.Fatalf("event %d = %s %s", want, ev.Method, ev.Params)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d not received", want)
		}
	}
}

func TestServerCloseReasonIsExposed(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})
	if !srv.WaitConnected(time.Second) {
		t.Fatal("client did not connect")
	}

	srv.CloseAll(websocket.StatusNormalClosure, "[NEW_DEBUGGER_OPENED] New debugger opened for the same app instance")

	select {
	case <-conn.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("connection did not report closure")
	}
	reason := conn.Reason()
	if reason.Code != cdp.CloseNewDebuggerOpened {
		t.Fatalf("Reason.Code = %q, want %q", reason.Code, cdp.CloseNewDebuggerOpened)
	}
	if reason.Message != "New debugger opened for the same app instance" {
		t.Fatalf("Reason.Message = %q", reason.Message)
	}
	if _, err := conn.Call(context.Background(), "Runtime.enable", nil); err == nil {
		t.Fatal("Call after close succeeded, want error")
	}
	if _, open := <-conn.Events(); open {
		t.Fatal("Events channel still open after close")
	}
}

func TestCallHonoursContextCancellation(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle("Slow", func(cdptest.Request) (any, *cdptest.Error) {
		time.Sleep(500 * time.Millisecond)
		return nil, nil
	})
	conn := dial(t, srv, cdpws.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := conn.Call(ctx, "Slow", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline", err)
	}
}

func TestCloseIsIdempotentAndReportsErrClosed(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})

	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	<-conn.Done()
	if !errors.Is(conn.Err(), cdpws.ErrClosed) {
		t.Fatalf("Err = %v, want ErrClosed", conn.Err())
	}
	want := cdp.CloseReason{Status: int(websocket.StatusNormalClosure), Message: "closed by user"}
	if got := conn.Reason(); got != want {
		t.Fatalf("Reason after Close = %+v, want %+v", got, want)
	}
}

func TestOnFrameObservesBothDirections(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	var mu sync.Mutex
	var dirs []cdp.Direction
	conn := dial(t, srv, cdpws.Options{OnFrame: func(dir cdp.Direction, _ []byte) {
		mu.Lock()
		dirs = append(dirs, dir)
		mu.Unlock()
	}})

	_, _ = conn.Call(context.Background(), "Anything", nil)

	mu.Lock()
	defer mu.Unlock()
	if len(dirs) != 2 || dirs[0] != cdp.DirectionSend || dirs[1] != cdp.DirectionReceive {
		t.Fatalf("observed %v, want [send recv]", dirs)
	}
}

func TestParseCloseReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in       string
		wantCode string
		wantMsg  string
	}{
		{"[CONNECTION_LOST] Connection lost", "CONNECTION_LOST", "Connection lost"},
		{"[PAGE_REMOVED]", "PAGE_REMOVED", ""},
		{"plain reason", "", "plain reason"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got := cdp.ParseCloseReason(1000, tt.in)
			if got.Code != tt.wantCode || got.Message != tt.wantMsg {
				t.Fatalf("ParseCloseReason(%q) = %+v", tt.in, got)
			}
		})
	}
}

func TestMalformedFrameIsSkipped(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})
	if !srv.WaitConnected(time.Second) {
		t.Fatal("client did not connect")
	}

	srv.EmitRaw([]byte(`{"method": "Runtime.consoleAPICalled", "params": {`))
	srv.Emit("Runtime.consoleAPICalled", map[string]any{"type": "log"})

	select {
	case ev := <-conn.Events():
		if ev.Method != "Runtime.consoleAPICalled" {
			t.Fatalf("event after malformed frame = %s, want Runtime.consoleAPICalled", ev.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event after malformed frame not received")
	}
	if _, err := conn.Call(context.Background(), "Runtime.enable", nil); !errors.As(err, new(*cdp.Error)) {
		t.Fatalf("Call after malformed frame = %v, want the fake's method-not-found error", err)
	}
	select {
	case <-conn.Done():
		t.Fatalf("connection ended after a malformed frame: %v", conn.Err())
	default:
	}
}

func TestOversizedFrameEndsTheConnection(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	srv.Handle("Network.getResponseBody", func(cdptest.Request) (any, *cdptest.Error) {
		return map[string]string{"body": strings.Repeat("x", 4096)}, nil
	})
	conn := dial(t, srv, cdpws.Options{MaxFrameBytes: 1024})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Call(ctx, "Network.getResponseBody", nil); err == nil {
		t.Fatal("Call returned an oversized result, want an error")
	}
	select {
	case <-conn.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after an oversized frame")
	}
	if conn.Err() == nil {
		t.Fatal("Err = nil after an oversized frame")
	}
	reason := conn.Reason()
	if reason.Status != -1 || reason.Code != "" || !strings.Contains(reason.Message, "message too big") {
		t.Fatalf("Reason = %+v, want status -1 and the read-limit error as the message", reason)
	}
}

func TestTransportErrorIsTheCloseReason(t *testing.T) {
	t.Parallel()
	srv := cdptest.New(t)
	conn := dial(t, srv, cdpws.Options{})
	if !srv.WaitConnected(time.Second) {
		t.Fatal("client did not connect")
	}

	srv.DropAll()

	select {
	case <-conn.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("connection did not report the dropped socket")
	}
	reason := conn.Reason()
	if reason.Code != "" || reason.Message == "" {
		t.Fatalf("Reason = %+v, want no code and the transport error as the message", reason)
	}
	if reason.Status != -1 {
		t.Fatalf("Reason.Status = %d, want -1 (no close frame)", reason.Status)
	}
	if err := conn.Err(); err == nil || !strings.Contains(err.Error(), reason.Message) {
		t.Fatalf("Err = %v, want it to carry %q", err, reason.Message)
	}
}
