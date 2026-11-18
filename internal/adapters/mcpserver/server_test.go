package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
)

func connect(t *testing.T, lens Lens) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	server := NewServer(lens, Options{Version: "test"})
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call runs a tool and returns its text, or fails when the tool failed and
// wantErr is false.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String(), res.IsError
}

func mustCall(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	text, isErr := call(t, cs, name, args)
	if isErr {
		t.Fatalf("%s failed: %s", name, text)
	}
	return text
}

func oneDevice() *fakeLens {
	return &fakeLens{devices: []queries.Device{device("dev.rnlens.example", "d1", "queries", "storage", "plugins")}}
}

func TestToolsAreListedWithTheirHints(t *testing.T) {
	t.Parallel()
	cs := connect(t, oneDevice())
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	read := []string{
		"status", "list_targets", "console_logs", "network_requests", "network_request", "list_queries",
		"get_query", "list_mutations", "list_stores", "get_storage_value", "list_plugins", "take_screenshot",
	}
	destructive := []string{"evaluate_js", "query_action", "set_storage_value", "remove_storage_key", "clear_storage", "plugin_action", "reload_app"}
	changing := []string{"attach_target", "set_online"}
	if len(res.Tools) != len(read)+len(destructive)+len(changing) {
		t.Fatalf("%d tools listed", len(res.Tools))
	}
	for _, tool := range res.Tools {
		a := tool.Annotations
		switch {
		case slices.Contains(read, tool.Name):
			if a == nil || !a.ReadOnlyHint {
				t.Errorf("%s is not read-only", tool.Name)
			}
		case slices.Contains(destructive, tool.Name):
			if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Errorf("%s is not destructive", tool.Name)
			}
		case slices.Contains(changing, tool.Name):
			if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
				t.Errorf("%s should change without being destructive", tool.Name)
			}
		default:
			t.Errorf("unexpected tool %s", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
}

func TestStatusAndConsole(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for i, e := range []console.Entry{
		{Kind: console.KindLog, Level: console.LevelInfo, Text: "boot"},
		{Kind: console.KindError, Level: console.LevelError, Text: "TypeError: x is undefined"},
		{Kind: console.KindLog, Level: console.LevelInfo, Text: "fetched users"},
		{Kind: console.KindWarning, Level: console.LevelWarning, Text: "slow render"},
	} {
		e.Seq, e.Timestamp = uint64(i+1), now
		lens.console = append(lens.console, e)
	}
	cs := connect(t, lens)
	if out := mustCall(t, cs, "status", nil); !strings.Contains(out, `"version": "9.9.9"`) || !strings.Contains(out, "dev.rnlens.example|d1") {
		t.Fatalf("status = %s", out)
	}
	var lines []struct{ Text string }
	if err := json.Unmarshal([]byte(mustCall(t, cs, "console_logs", map[string]any{"level": "error"})), &lines); err != nil || len(lines) != 1 || !strings.HasPrefix(lines[0].Text, "TypeError") {
		t.Fatalf("errors = %+v (%v)", lines, err)
	}
	if err := json.Unmarshal([]byte(mustCall(t, cs, "console_logs", map[string]any{"search": "USERS"})), &lines); err != nil || len(lines) != 1 {
		t.Fatalf("search = %+v", lines)
	}
	if err := json.Unmarshal([]byte(mustCall(t, cs, "console_logs", map[string]any{"limit": 2})), &lines); err != nil || len(lines) != 2 || lines[1].Text != "slow render" {
		t.Fatalf("limit = %+v", lines)
	}
}

func TestNetwork(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	lens.reqs = []network.Summary{{Seq: 3, URL: "https://api.example.com/users"}, {Seq: 2, URL: "https://cdn.example.com/a.png"}, {Seq: 1, URL: "https://api.example.com/login"}}
	cs := connect(t, lens)
	var rows []network.Summary
	if err := json.Unmarshal([]byte(mustCall(t, cs, "network_requests", map[string]any{"search": "API.example", "limit": 1})), &rows); err != nil || len(rows) != 1 || rows[0].Seq != 3 {
		t.Fatalf("rows = %+v (%v)", rows, err)
	}
	out := mustCall(t, cs, "network_request", map[string]any{"seq": 3})
	if !strings.Contains(out, `"requestBody": "{\"q\":1}"`) || !strings.Contains(out, `{\"ok\":true}`) {
		t.Fatalf("request = %s", out)
	}
	if out, isErr := call(t, cs, "network_request", map[string]any{"seq": 99}); !isErr || !strings.Contains(out, "no request with seq 99") {
		t.Fatalf("unknown seq = %s", out)
	}
}

func TestDevicesAreResolved(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	cs := connect(t, lens)
	mustCall(t, cs, "list_queries", nil)
	mustCall(t, cs, "list_queries", map[string]any{"device": "dev.rnlens.example"})
	if out, isErr := call(t, cs, "list_queries", map[string]any{"device": "other.app"}); !isErr || !strings.Contains(out, `unknown device "other.app"`) {
		t.Fatalf("unknown device = %s", out)
	}
	if got := lens.Calls(); !slices.Equal(got, []string{"queries:dev.rnlens.example|d1", "queries:dev.rnlens.example|d1"}) {
		t.Fatalf("calls = %v", got)
	}

	two := oneDevice()
	two.devices = append(two.devices, device("dev.rnlens.example", "d2", "queries"))
	cs2 := connect(t, two)
	if out, isErr := call(t, cs2, "list_queries", nil); !isErr || !strings.Contains(out, "several devices are connected") {
		t.Fatalf("ambiguous = %s", out)
	}
	if out, isErr := call(t, cs2, "list_queries", map[string]any{"device": "dev.rnlens.example"}); !isErr || !strings.Contains(out, "matches several devices") {
		t.Fatalf("an app id matching two devices = %s", out)
	}
	mustCall(t, cs2, "list_queries", map[string]any{"device": "dev.rnlens.example|d2"})

	none := &fakeLens{}
	if out, isErr := call(t, connect(t, none), "list_stores", nil); !isErr || !strings.Contains(out, "no app with @rnlens/sdk") {
		t.Fatalf("no device = %s", out)
	}
}

func TestQueryTools(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	lens.queries = []queries.Query{{Query: protocol.Query{QueryHash: `["users"]`, QueryKey: json.RawMessage(`["users"]`), State: protocol.QueryState{Status: "success", Data: json.RawMessage(`[{"id":1}]`)}}, Label: queries.LabelStale}}
	cs := connect(t, lens)
	if out := mustCall(t, cs, "list_queries", nil); !strings.Contains(out, `"label": "stale"`) || strings.Contains(out, `"id": 1`) {
		t.Fatalf("list = %s", out)
	}
	if out := mustCall(t, cs, "get_query", map[string]any{"queryHash": `["users"]`}); !strings.Contains(out, `"id": 1`) {
		t.Fatalf("get = %s", out)
	}
	if out, isErr := call(t, cs, "query_action", map[string]any{"action": "EXPLODE", "queryHash": "x"}); !isErr || !strings.Contains(out, `unknown action "EXPLODE"`) {
		t.Fatalf("bad action = %s", out)
	}
	if out, isErr := call(t, cs, "query_action", map[string]any{"action": "SET_QUERY_DATA", "queryHash": "x"}); !isErr || !strings.Contains(out, "needs data") {
		t.Fatalf("no data = %s", out)
	}
	mustCall(t, cs, "query_action", map[string]any{"action": "SET_QUERY_DATA", "queryHash": `["users"]`, "data": []any{map[string]any{"id": 2}}})
	mustCall(t, cs, "query_action", map[string]any{"action": "INVALIDATE", "queryHash": `["users"]`})
	mustCall(t, cs, "set_online", map[string]any{"online": false})
	want := []string{
		"queries:dev.rnlens.example|d1",
		`action:dev.rnlens.example|d1:SET_QUERY_DATA:["users"]:[{"id":2}]`,
		`action:dev.rnlens.example|d1:INVALIDATE:["users"]:`,
		"online:dev.rnlens.example|d1:off",
	}
	if got := lens.Calls(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v", got)
	}
}

func TestStorageAndPluginTools(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	lens.stores = []protocol.StorageStore{{ID: "async-storage", Kind: "async-storage", Name: "AsyncStorage"}}
	cs := connect(t, lens)
	if out := mustCall(t, cs, "list_stores", nil); !strings.Contains(out, `"async-storage"`) {
		t.Fatalf("stores = %s", out)
	}
	if out := mustCall(t, cs, "get_storage_value", map[string]any{"store": "async-storage", "key": "theme"}); !strings.Contains(out, "async-storage/theme") {
		t.Fatalf("get = %s", out)
	}
	mustCall(t, cs, "set_storage_value", map[string]any{"store": "async-storage", "key": "launches", "type": "number", "value": "42"})
	if out, isErr := call(t, cs, "set_storage_value", map[string]any{"store": "async-storage", "key": "n", "type": "number", "value": "forty"}); !isErr {
		t.Fatalf("a number that is not a number was written: %s", out)
	}
	mustCall(t, cs, "set_storage_value", map[string]any{"store": "async-storage", "key": "theme", "value": "dark"})
	mustCall(t, cs, "remove_storage_key", map[string]any{"store": "async-storage", "key": "theme"})
	mustCall(t, cs, "clear_storage", map[string]any{"store": "async-storage"})
	if out := mustCall(t, cs, "plugin_action", map[string]any{"plugin": "feature-flags", "block": "flags", "action": "toggle", "row": "darkMode"}); out != "darkMode is on" {
		t.Fatalf("plugin = %q", out)
	}
	want := []string{
		"set:dev.rnlens.example|d1:async-storage:launches",
		"set:dev.rnlens.example|d1:async-storage:theme",
		"remove:dev.rnlens.example|d1:async-storage:theme",
		"clear:dev.rnlens.example|d1:async-storage",
		"plugin:dev.rnlens.example|d1:feature-flags:flags:toggle:darkMode",
	}
	if got := lens.Calls(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v", got)
	}
	if lens.set[0] != (protocol.StorageValue{Type: "number", Value: "42"}) || lens.set[1].Type != "string" {
		t.Fatalf("values = %+v", lens.set)
	}
}

func TestEvaluateScreenshotAndAppControl(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	cs := connect(t, lens)
	if out := mustCall(t, cs, "evaluate_js", map[string]any{"expression": "1+1"}); !strings.Contains(out, `"value": 2`) {
		t.Fatalf("eval = %s", out)
	}
	lens.evalErr = true
	if out, isErr := call(t, cs, "evaluate_js", map[string]any{"expression": "nope"}); !isErr || !strings.Contains(out, "ReferenceError") {
		t.Fatalf("throwing eval = %s", out)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "take_screenshot"})
	if err != nil || len(res.Content) != 1 {
		t.Fatalf("screenshot = %+v, %v", res, err)
	}
	if img, ok := res.Content[0].(*mcp.ImageContent); !ok || img.MIMEType != "image/png" || string(img.Data) != "\x89PNG" {
		t.Fatalf("screenshot content = %#v", res.Content[0])
	}
	mustCall(t, cs, "attach_target", map[string]any{"targetId": "t1"})
	mustCall(t, cs, "reload_app", nil)
	if got := lens.Calls(); !slices.Equal(got, []string{"eval:1+1", "eval:nope", "attach:t1", "reload"}) {
		t.Fatalf("calls = %v", got)
	}
}

func TestLargeResultsAreCut(t *testing.T) {
	t.Parallel()
	res, _, err := result(strings.Repeat("x", maxResultBytes*2))
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if len(text) > maxResultBytes+200 || !strings.HasSuffix(text, "narrow the request with search or limit)") {
		t.Fatalf("cut text is %d bytes", len(text))
	}
}

func TestEndpointOverHTTP(t *testing.T) {
	t.Parallel()
	ep, err := Start(oneDevice(), 0, Options{Token: "s3cret", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ep.Close() })
	if !strings.HasPrefix(ep.URL, "http://127.0.0.1:") || !strings.HasSuffix(ep.URL, "/mcp") {
		t.Fatalf("URL = %s", ep.URL)
	}
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	post := func(headers map[string]string) int {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ep.URL, strings.NewReader(initialize))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(nil); code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", code)
	}
	if code := post(map[string]string{"Authorization": "Bearer wrong"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", code)
	}
	if code := post(map[string]string{"Authorization": "Bearer s3cret", "Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}); code != http.StatusForbidden {
		t.Fatalf("cross-origin: %d", code)
	}

	// A real client session over HTTP.
	transport := &mcp.StreamableClientTransport{Endpoint: ep.URL, HTTPClient: &http.Client{Transport: bearer{"s3cret"}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	if out := mustCall(t, cs, "status", nil); !strings.Contains(out, "dev.rnlens.example|d1") {
		t.Fatalf("status over HTTP = %s", out)
	}
}

func TestEndpointRefusesForeignHosts(t *testing.T) {
	t.Parallel()
	h := Handler(NewServer(oneDevice(), Options{}), Options{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://evil.example/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Fatalf("a request for host evil.example got %d", rec.Code)
	}
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
