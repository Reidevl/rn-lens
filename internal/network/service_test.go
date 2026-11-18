package network_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws"
	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

type harness struct {
	srv      *cdptest.Server
	svc      *network.Service
	notified *atomic.Int64
}

func newHarness(t *testing.T, opts network.ServiceOptions) *harness {
	t.Helper()
	srv := cdptest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cdpws.Dial(ctx, srv.URL(), cdpws.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var notified atomic.Int64
	opts.Notify = func() { notified.Add(1) }
	svc := network.NewService(network.NewStore(0, nil), conn, opts)
	return &harness{srv: srv, svc: svc, notified: &notified}
}

func ok(cdptest.Request) (any, *cdptest.Error) { return map[string]any{}, nil }

func (h *harness) count(method string) int {
	n := 0
	for _, m := range h.srv.Methods() {
		if m == method {
			n++
		}
	}
	return n
}

// finished feeds a complete request through the service and returns its seq.
func (h *harness) finished(t *testing.T, id string, req map[string]any) uint64 {
	t.Helper()
	if req == nil {
		req = map[string]any{"url": "https://x/" + id, "method": "GET", "headers": map[string]any{}}
	}
	h.svc.HandleEvent(event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{"requestId": id, "request": req, "timestamp": 1}))
	h.svc.HandleEvent(event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": id, "timestamp": 2}))
	rows := h.svc.Store().Since(0, 0)
	return rows[len(rows)-1].Seq
}

func TestOnAttachedEnablesNetworkAndReportsSupport(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.srv.Handle(cdp.MethodNetworkEnable, ok)

	if err := h.svc.OnAttached(context.Background()); err != nil {
		t.Fatalf("OnAttached: %v", err)
	}
	if got := h.svc.Availability(); !got.Checked || !got.Supported || got.Reason != "" {
		t.Fatalf("Availability = %+v, want supported", got)
	}
	if h.count(cdp.MethodNetworkEnable) != 1 {
		t.Fatalf("methods = %v", h.srv.Methods())
	}
	before := h.notified.Load()
	h.svc.HandleEvent(willBeSend(t, "1", "https://x/1", 1))
	if h.notified.Load() <= before {
		t.Fatal("a stored event did not notify")
	}
	before = h.notified.Load()
	h.svc.HandleEvent(cdp.Event{Method: cdp.EventRuntimeConsoleAPICalled, Params: json.RawMessage(`{}`)})
	if h.notified.Load() != before {
		t.Fatal("an ignored event notified")
	}
}

func TestOnAttachedExplainsWhyNetworkIsUnavailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  *cdptest.Error
		want string
	}{
		{"old React Native", &cdptest.Error{Code: -32601, Message: "Unsupported method 'Network.enable'"}, network.ReasonNeedsNewerReactNative},
		{"other failure", &cdptest.Error{Code: -32000, Message: "Multiple hosts registered"}, "Network.enable failed: Multiple hosts registered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, network.ServiceOptions{})
			h.srv.Handle(cdp.MethodNetworkEnable, func(cdptest.Request) (any, *cdptest.Error) { return nil, tt.err })
			if err := h.svc.OnAttached(context.Background()); err != nil {
				t.Fatalf("OnAttached returned %v; an unsupported target is not an error", err)
			}
			got := h.svc.Availability()
			if !got.Checked || got.Supported || got.Reason != tt.want {
				t.Fatalf("Availability = %+v, want reason %q", got, tt.want)
			}
		})
	}
}

func TestResponseBodiesAreFetchedOnceAndCached(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.srv.Handle(cdp.MethodNetworkGetResponseBody, func(r cdptest.Request) (any, *cdptest.Error) {
		var p cdp.GetResponseBodyParams
		_ = json.Unmarshal(r.Params, &p)
		if p.RequestID == "img" {
			return cdp.GetResponseBodyResult{Body: "iVBORw0KGgo=", Base64Encoded: true}, nil
		}
		return cdp.GetResponseBodyResult{Body: `{"slideshow":{}}`}, nil
	})
	seq := h.finished(t, "json", nil)
	epoch := h.svc.Store().Epoch()

	for range 2 {
		body, err := h.svc.ResponseBody(context.Background(), seq, epoch)
		if err != nil {
			t.Fatalf("ResponseBody: %v", err)
		}
		if body.Text != `{"slideshow":{}}` || body.Base64 || body.Truncated || body.Size != 16 {
			t.Fatalf("body = %+v", body)
		}
	}
	if n := h.count(cdp.MethodNetworkGetResponseBody); n != 1 {
		t.Fatalf("getResponseBody called %d times, want 1 (cached)", n)
	}

	img, err := h.svc.ResponseBody(context.Background(), h.finished(t, "img", nil), epoch)
	if err != nil || !img.Base64 || img.Text != "iVBORw0KGgo=" {
		t.Fatalf("image body = %+v, %v", img, err)
	}
}

func TestBodiesAreRefusedAfterReattachAndForUnknownRequests(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.srv.Handle(cdp.MethodNetworkEnable, ok)
	h.srv.Handle(cdp.MethodNetworkGetResponseBody, func(cdptest.Request) (any, *cdptest.Error) {
		return cdp.GetResponseBodyResult{Body: "x"}, nil
	})
	if err := h.svc.OnAttached(context.Background()); err != nil {
		t.Fatal(err)
	}
	seq := h.finished(t, "1", nil)
	oldEpoch := h.svc.Store().Epoch()

	if err := h.svc.OnAttached(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.svc.Store().Epoch() != oldEpoch+1 {
		t.Fatalf("re-attach did not start a new epoch")
	}
	if _, err := h.svc.ResponseBody(context.Background(), seq, oldEpoch); !errors.Is(err, network.ErrStale) {
		t.Fatalf("ResponseBody(old epoch) err = %v, want ErrStale", err)
	}
	if _, err := h.svc.ResponseBody(context.Background(), 999, h.svc.Store().Epoch()); !errors.Is(err, network.ErrNotFound) {
		t.Fatalf("ResponseBody(unknown) err = %v, want ErrNotFound", err)
	}
	if n := h.count(cdp.MethodNetworkGetResponseBody); n != 0 {
		t.Fatalf("refused requests still reached the app %d times", n)
	}
}

func TestOversizedBodiesAndTheCacheBudget(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{MaxBodyBytes: 10, CacheBytes: 25})
	h.srv.Handle(cdp.MethodNetworkGetResponseBody, func(r cdptest.Request) (any, *cdptest.Error) {
		var p cdp.GetResponseBodyParams
		_ = json.Unmarshal(r.Params, &p)
		if p.RequestID == "huge" {
			return cdp.GetResponseBodyResult{Body: strings.Repeat("h", 11)}, nil
		}
		return cdp.GetResponseBodyResult{Body: strings.Repeat(p.RequestID, 10)}, nil
	})
	epoch := h.svc.Store().Epoch()
	ctx := context.Background()

	huge, err := h.svc.ResponseBody(ctx, h.finished(t, "huge", nil), epoch)
	if err != nil || !huge.Truncated || huge.Text != "" || huge.Size != 11 {
		t.Fatalf("huge body = %+v, %v; want truncated with its size", huge, err)
	}

	a, b, c := h.finished(t, "a", nil), h.finished(t, "b", nil), h.finished(t, "c", nil)
	for _, seq := range []uint64{a, b, c} {
		if _, err := h.svc.ResponseBody(ctx, seq, epoch); err != nil {
			t.Fatal(err)
		}
	}
	calls := h.count(cdp.MethodNetworkGetResponseBody)
	if _, err := h.svc.ResponseBody(ctx, c, epoch); err != nil {
		t.Fatal(err)
	}
	if h.count(cdp.MethodNetworkGetResponseBody) != calls {
		t.Fatal("the most recent body was evicted")
	}
	if _, err := h.svc.ResponseBody(ctx, a, epoch); err != nil {
		t.Fatal(err)
	}
	if h.count(cdp.MethodNetworkGetResponseBody) != calls+1 {
		t.Fatal("the oldest body stayed cached beyond the byte budget")
	}
}

func TestRequestBodies(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.srv.Handle(cdp.MethodNetworkGetRequestPostData, func(cdptest.Request) (any, *cdptest.Error) {
		return cdp.GetRequestPostDataResult{PostData: "fetched=1"}, nil
	})
	epoch := h.svc.Store().Epoch()
	ctx := context.Background()

	inline := h.finished(t, "inline", map[string]any{"url": "https://x", "method": "POST", "headers": map[string]any{}, "postData": "a=1", "hasPostData": true})
	lazy := h.finished(t, "lazy", map[string]any{"url": "https://x", "method": "POST", "headers": map[string]any{}, "hasPostData": true})
	none := h.finished(t, "none", nil)

	if got, err := h.svc.RequestBody(ctx, inline, epoch); err != nil || got != "a=1" {
		t.Fatalf("inline = %q, %v", got, err)
	}
	if got, err := h.svc.RequestBody(ctx, none, epoch); err != nil || got != "" {
		t.Fatalf("none = %q, %v", got, err)
	}
	if n := h.count(cdp.MethodNetworkGetRequestPostData); n != 0 {
		t.Fatalf("inline or empty bodies reached the app (%d calls)", n)
	}
	if got, err := h.svc.RequestBody(ctx, lazy, epoch); err != nil || got != "fetched=1" {
		t.Fatalf("lazy = %q, %v", got, err)
	}
}

func TestClearEmptiesTheStoreAndNotifies(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.finished(t, "1", nil)
	before := h.notified.Load()
	h.svc.Clear()
	if h.svc.Store().Stats().Len != 0 || h.notified.Load() <= before {
		t.Fatalf("Clear: len=%d notified=%v", h.svc.Store().Stats().Len, h.notified.Load() > before)
	}
}
