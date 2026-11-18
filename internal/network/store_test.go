package network_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/platform/clock"
)

func event(t *testing.T, method string, params any) cdp.Event {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return cdp.Event{Method: method, Params: raw}
}

func newStore(capacity int) *network.Store {
	return network.NewStore(capacity, clock.NewFake(time.Unix(1_700_000_000, 0)))
}

func willBeSend(t *testing.T, id, url string, ts float64) cdp.Event {
	t.Helper()
	return event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{
		"requestId": id,
		"request":   map[string]any{"url": url, "method": "GET", "headers": map[string]any{"Accept": "application/json", "X-Retry": 2}},
		"timestamp": ts,
		"wallTime":  1_700_000_100.5,
		"type":      "Fetch",
		"initiator": map[string]any{"type": "script", "url": "index.bundle"},
	})
}

func ingest(t *testing.T, s *network.Store, evs ...cdp.Event) {
	t.Helper()
	for _, ev := range evs {
		if !s.Ingest(ev) {
			t.Fatalf("Ingest(%s) = false", ev.Method)
		}
	}
}

func onlyRequest(t *testing.T, s *network.Store) network.Request {
	t.Helper()
	rows := s.Since(0, 0)
	if len(rows) != 1 {
		t.Fatalf("Since(0) = %d rows, want 1", len(rows))
	}
	req, ok := s.Get(rows[0].Seq)
	if !ok {
		t.Fatalf("Get(%d) missing", rows[0].Seq)
	}
	return req
}

func TestRequestLifecycleIsCorrelatedByRequestID(t *testing.T) {
	s := newStore(0)
	ingest(t, s,
		willBeSend(t, "1", "https://httpbin.org/json", 10),
		event(t, cdp.EventNetworkResponseReceived, map[string]any{
			"requestId": "1", "timestamp": 10.2, "type": "Fetch",
			"response": map[string]any{
				"url": "https://httpbin.org/json", "status": 200, "statusText": "OK",
				"headers": map[string]any{"Content-Type": "application/json"}, "mimeType": "application/json",
				"protocol": "h2", "remoteIPAddress": "1.2.3.4", "remotePort": 443,
				"timing": map[string]any{"requestTime": 10, "dnsStart": 1, "dnsEnd": 3, "sendStart": 5, "sendEnd": 6, "receiveHeadersEnd": 180},
			},
		}),
		event(t, cdp.EventNetworkDataReceived, map[string]any{"requestId": "1", "timestamp": 10.3, "dataLength": 300, "encodedDataLength": 120}),
		event(t, cdp.EventNetworkDataReceived, map[string]any{"requestId": "1", "timestamp": 10.4, "dataLength": 129, "encodedDataLength": 80}),
		event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": "1", "timestamp": 10.5, "encodedDataLength": 512}),
	)

	req := onlyRequest(t, s)
	if req.URL != "https://httpbin.org/json" || req.Method != http.MethodGet || req.ResourceType != "Fetch" {
		t.Fatalf("request line = %s %s (%s)", req.Method, req.URL, req.ResourceType)
	}
	if req.State != network.StateComplete || req.Status != 200 || req.StatusText != "OK" || req.MimeType != "application/json" {
		t.Fatalf("response = %+v", req.Summary)
	}
	if req.DecodedSize != 429 || req.EncodedSize != 512 {
		t.Fatalf("sizes decoded=%d encoded=%d, want 429/512", req.DecodedSize, req.EncodedSize)
	}
	if req.StartTime != 10 || req.ResponseTime != 10.2 || req.EndTime != 10.5 {
		t.Fatalf("times = %v/%v/%v", req.StartTime, req.ResponseTime, req.EndTime)
	}
	if req.StartedAt != 1_700_000_100_500 {
		t.Fatalf("StartedAt = %d, want wall time in ms", req.StartedAt)
	}
	if req.RequestHeaders["X-Retry"] != "2" || req.ResponseHeaders["Content-Type"] != "application/json" {
		t.Fatalf("headers req=%v resp=%v", req.RequestHeaders, req.ResponseHeaders)
	}
	if req.Protocol != "h2" || req.RemoteAddress != "1.2.3.4:443" || req.Timing == nil || req.Timing.ReceiveHeadersEnd != 180 {
		t.Fatalf("connection = %q %q %+v", req.Protocol, req.RemoteAddress, req.Timing)
	}
	if req.Initiator == nil || req.Initiator.Type != "script" {
		t.Fatalf("initiator = %+v", req.Initiator)
	}
	if req.Epoch != 1 {
		t.Fatalf("Epoch = %d, want 1", req.Epoch)
	}
}

func TestSinceReturnsOnlyChangedRowsInVersionOrder(t *testing.T) {
	s := newStore(0)
	ingest(t, s, willBeSend(t, "a", "https://x/a", 1), willBeSend(t, "b", "https://x/b", 2))
	mark := s.Stats().Version

	ingest(t, s, event(t, cdp.EventNetworkLoadingFailed, map[string]any{"requestId": "a", "timestamp": 3, "errorText": "net::ERR_FAILED"}))

	changed := s.Since(mark, 0)
	if len(changed) != 1 || changed[0].RequestID != "a" || changed[0].Version != s.Stats().Version {
		t.Fatalf("Since(mark) = %+v", changed)
	}
	if got := s.Since(s.Stats().Version, 0); len(got) != 0 {
		t.Fatalf("Since(latest) = %d rows, want 0", len(got))
	}

	page := s.Since(0, 1)
	if len(page) != 1 || page[0].RequestID != "b" {
		t.Fatalf("first page = %+v, want the oldest version (b)", page)
	}
	rest := s.Since(page[0].Version, 0)
	if len(rest) != 1 || rest[0].RequestID != "a" {
		t.Fatalf("second page = %+v", rest)
	}
}

func TestFailedAndCanceledRequests(t *testing.T) {
	s := newStore(0)
	ingest(t, s,
		willBeSend(t, "1", "https://x/slow", 1),
		event(t, cdp.EventNetworkLoadingFailed, map[string]any{"requestId": "1", "timestamp": 4, "errorText": "cancelled", "canceled": true}),
	)
	req := onlyRequest(t, s)
	if req.State != network.StateFailed || req.ErrorText != "cancelled" || !req.Canceled || req.EndTime != 4 {
		t.Fatalf("failed request = %+v", req.Summary)
	}
}

func TestRedirectClosesThePreviousHopAndOpensANewOne(t *testing.T) {
	s := newStore(0)
	ingest(t, s, willBeSend(t, "r", "http://x/old", 1))
	redirect := event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{
		"requestId": "r",
		"request":   map[string]any{"url": "https://x/new", "method": "GET", "headers": map[string]any{}},
		"timestamp": 2, "wallTime": 1_700_000_101,
		"redirectResponse": map[string]any{"url": "http://x/old", "status": 301, "statusText": "Moved", "headers": map[string]any{"Location": "https://x/new"}, "mimeType": ""},
	})
	ingest(t, s, redirect,
		event(t, cdp.EventNetworkResponseReceived, map[string]any{"requestId": "r", "timestamp": 3, "response": map[string]any{"url": "https://x/new", "status": 200, "headers": map[string]any{}, "mimeType": "text/plain"}}),
	)

	rows := s.Since(0, 0)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 hops", len(rows))
	}
	byURL := map[string]network.Summary{}
	for _, r := range rows {
		byURL[r.URL] = r
	}
	old, next := byURL["http://x/old"], byURL["https://x/new"]
	if !old.Redirected || old.Status != 301 || old.State != network.StateComplete || old.EndTime != 2 {
		t.Fatalf("old hop = %+v", old)
	}
	if next.Redirected || next.Status != 200 || next.State != network.StatePending || next.Seq <= old.Seq {
		t.Fatalf("new hop = %+v", next)
	}
}

func TestCapacityEvictsTheOldestRequest(t *testing.T) {
	s := newStore(2)
	ingest(t, s, willBeSend(t, "1", "https://x/1", 1), willBeSend(t, "2", "https://x/2", 2), willBeSend(t, "3", "https://x/3", 3))

	st := s.Stats()
	if st.Len != 2 || st.Dropped != 1 || st.OldestSeq != 2 || st.LastSeq != 3 {
		t.Fatalf("stats = %+v", st)
	}
	if _, ok := s.Get(1); ok {
		t.Fatal("evicted request still readable")
	}
	if s.Ingest(event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": "1", "timestamp": 5})) {
		t.Fatal("event for an evicted request was applied")
	}
}

func TestClearDropsRequestsAndIgnoresTheirLateEvents(t *testing.T) {
	s := newStore(0)
	ingest(t, s, willBeSend(t, "1", "https://x/1", 1))
	s.Clear()

	st := s.Stats()
	if st.Len != 0 || st.Cleared != 1 {
		t.Fatalf("stats after clear = %+v", st)
	}
	if s.Ingest(event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": "1", "timestamp": 2})) {
		t.Fatal("late event for a cleared request was applied")
	}
	if rows := s.Since(0, 0); len(rows) != 0 {
		t.Fatalf("Since after clear = %d rows", len(rows))
	}
	ingest(t, s, willBeSend(t, "2", "https://x/2", 3))
	if rows := s.Since(0, 0); len(rows) != 1 || rows[0].Seq != 2 {
		t.Fatalf("rows after clear = %+v, want seq 2 (sequence keeps growing)", rows)
	}
}

func TestNewEpochTagsLaterRequests(t *testing.T) {
	s := newStore(0)
	ingest(t, s, willBeSend(t, "1", "https://x/1", 1))
	if got := s.NewEpoch(); got != 2 {
		t.Fatalf("NewEpoch = %d, want 2", got)
	}
	ingest(t, s, willBeSend(t, "2", "https://x/2", 2))
	rows := s.Since(0, 0)
	if rows[0].Epoch != 1 || rows[1].Epoch != 2 {
		t.Fatalf("epochs = %d, %d", rows[0].Epoch, rows[1].Epoch)
	}
}

func TestEventsTheStoreCannotUseAreIgnored(t *testing.T) {
	s := newStore(0)
	cases := []cdp.Event{
		{Method: cdp.EventRuntimeConsoleAPICalled, Params: json.RawMessage(`{}`)},
		{Method: cdp.EventNetworkRequestWillBeSent, Params: json.RawMessage(`{"requestId":`)},
		{Method: cdp.EventNetworkRequestWillBeSent, Params: json.RawMessage(`{"request":{"url":"https://x"}}`)},
		event(t, cdp.EventNetworkResponseReceived, map[string]any{"requestId": "unknown", "response": map[string]any{"status": 200}}),
	}
	for _, ev := range cases {
		if s.Ingest(ev) {
			t.Errorf("Ingest(%s %s) = true, want false", ev.Method, ev.Params)
		}
	}
	if s.Stats().Version != 0 {
		t.Fatalf("version moved to %d on ignored events", s.Stats().Version)
	}
}

func TestLargeInlinePostDataIsLeftForLazyFetch(t *testing.T) {
	s := newStore(0)
	big := strings.Repeat("x", network.MaxInlinePostData+1)
	ingest(t, s,
		event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{
			"requestId": "small", "timestamp": 1,
			"request": map[string]any{"url": "https://x/a", "method": "POST", "headers": map[string]any{}, "postData": `{"a":1}`, "hasPostData": true},
		}),
		event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{
			"requestId": "big", "timestamp": 2,
			"request": map[string]any{"url": "https://x/b", "method": "POST", "headers": map[string]any{}, "postData": big, "hasPostData": true},
		}),
	)
	rows := s.Since(0, 0)
	small, _ := s.Get(rows[0].Seq)
	large, _ := s.Get(rows[1].Seq)
	if small.PostData != `{"a":1}` || !small.HasPostData {
		t.Fatalf("small post = %q %v", small.PostData, small.HasPostData)
	}
	if large.PostData != "" || !large.HasPostData {
		t.Fatalf("large post kept inline (%d bytes) or lost its flag", len(large.PostData))
	}
}

func TestAReusedIDWithoutARedirectFailsTheEarlierPendingRow(t *testing.T) {
	s := newStore(0)
	ingest(t, s, willBeSend(t, "1", "https://x/first", 1), willBeSend(t, "1", "https://x/second", 2))
	rows := s.Since(0, 0)
	var first, second network.Summary
	for _, r := range rows {
		if r.URL == "https://x/first" {
			first = r
		} else {
			second = r
		}
	}
	if first.State != network.StateFailed || first.ErrorText != network.ErrorReplaced || first.Redirected {
		t.Fatalf("first = %+v", first)
	}
	if second.State != network.StatePending {
		t.Fatalf("second = %+v", second)
	}
}
