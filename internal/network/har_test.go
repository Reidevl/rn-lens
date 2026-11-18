package network_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Reidevl/rn-lens/internal/adapters/cdpws/cdptest"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
)

var update = flag.Bool("update", false, "rewrite golden files")

// harFixture stores a JSON GET with timing, a POST without timing that
// failed, and a binary response.
func harFixture(t *testing.T) (*network.Store, network.HARBodies) {
	t.Helper()
	s := newStore(0)
	ingest(t, s,
		willBeSend(t, "1", "https://httpbin.org/json?a=1&b=x%20y", 100),
		event(t, cdp.EventNetworkResponseReceived, map[string]any{
			"requestId": "1", "timestamp": 100.2,
			"response": map[string]any{
				"url": "https://httpbin.org/json", "status": 200, "statusText": "OK", "mimeType": "application/json",
				"headers": map[string]any{"Content-Type": "application/json", "Content-Length": "16", "Set-Cookie": "a=1\nb=2"}, "protocol": "h2",
				"remoteIPAddress": "2001:db8::1", "remotePort": 443,
				"timing": map[string]any{"requestTime": 100, "proxyStart": -1, "proxyEnd": -1, "dnsStart": 2, "dnsEnd": 5, "connectStart": 5, "connectEnd": 30, "sslStart": 10, "sslEnd": 30, "sendStart": 31, "sendEnd": 32, "receiveHeadersEnd": 180},
			},
		}),
		event(t, cdp.EventNetworkDataReceived, map[string]any{"requestId": "1", "timestamp": 100.25, "dataLength": 16, "encodedDataLength": 10}),
		event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": "1", "timestamp": 100.25, "encodedDataLength": 90}),
		event(t, cdp.EventNetworkRequestWillBeSent, map[string]any{
			"requestId": "2", "timestamp": 101, "wallTime": 1_700_000_101,
			"request": map[string]any{"url": "https://api.example.com/todos", "method": "POST", "headers": map[string]any{"Content-Type": "application/json"}, "postData": `{"title":"x"}`, "hasPostData": true},
		}),
		event(t, cdp.EventNetworkLoadingFailed, map[string]any{"requestId": "2", "timestamp": 101.5, "errorText": "net::ERR_CONNECTION_REFUSED"}),
		willBeSend(t, "3", "https://cdn.example.com/a.png", 102),
		event(t, cdp.EventNetworkResponseReceived, map[string]any{"requestId": "3", "timestamp": 102.1, "response": map[string]any{"url": "https://cdn.example.com/a.png", "status": 200, "statusText": "", "mimeType": "image/png", "headers": map[string]any{}}}),
		event(t, cdp.EventNetworkLoadingFinished, map[string]any{"requestId": "3", "timestamp": 102.3, "encodedDataLength": 8}),
	)
	rows := s.Since(0, 0)
	return s, network.HARBodies{
		Responses: map[uint64]network.Body{
			rows[0].Seq: {Text: `{"slideshow":{}}`, Size: 16},
			rows[2].Seq: {Text: "iVBORw0KGgo=", Base64: true, Size: 12},
		},
		Requests: map[uint64]string{rows[1].Seq: `{"title":"x"}`},
	}
}

func TestBuildHARMatchesGolden(t *testing.T) {
	s, bodies := harFixture(t)
	got, err := network.EncodeHAR(network.BuildHAR(s.All(), "1.2.3", bodies))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "export.har.golden.json")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("HAR differs from %s; run go test ./internal/network -run TestBuildHARMatchesGolden -update and review the diff\n%s", path, got)
	}
}

func TestExportHARFetchesBodiesOfTheCurrentSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, network.ServiceOptions{})
	h.srv.Handle(cdp.MethodNetworkGetResponseBody, func(cdptest.Request) (any, *cdptest.Error) {
		return cdp.GetResponseBodyResult{Body: "hello"}, nil
	})
	h.finished(t, "1", nil)

	raw, err := h.svc.ExportHAR(context.Background(), "dev")
	if err != nil {
		t.Fatalf("ExportHAR: %v", err)
	}
	var har network.HAR
	if err := json.Unmarshal(raw, &har); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	if har.Log.Version != "1.2" || har.Log.Creator.Name != "Lens" || len(har.Log.Entries) != 1 {
		t.Fatalf("log = %+v", har.Log)
	}
	if got := har.Log.Entries[0].Response.Content.Text; got != "hello" {
		t.Fatalf("content text = %q, want the fetched body", got)
	}
}
