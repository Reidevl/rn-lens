package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
)

// The level filter uses the names an AI knows (log, debug, warn) and the
// buckets the console store keeps (info, verbose, warning).
func TestConsoleLevelsMatchTheStore(t *testing.T) {
	t.Parallel()
	core := app.New(app.Config{SDKAddr: "off", MetroEndpoints: []string{"http://127.0.0.1:1"}})
	st := core.Console().Store()
	st.Append(console.Entry{Kind: console.KindLog, Text: "a log"})
	st.Append(console.Entry{Kind: console.KindDebug, Text: "a debug"})
	st.Append(console.Entry{Kind: console.KindWarning, Text: "a warning"})
	cs := connect(t, AppLens{App: core})
	for level, want := range map[string]string{
		"log": "a log", "info": "a log", "debug": "a debug", "verbose": "a debug", "warn": "a warning", "warning": "a warning",
	} {
		out := mustCall(t, cs, "console_logs", map[string]any{"level": level})
		if !strings.Contains(out, want) || strings.Count(out, `"text"`) != 1 {
			t.Errorf("level=%s returned %s, want only %q", level, out, want)
		}
	}
	if out, isErr := call(t, cs, "console_logs", map[string]any{"level": "loud"}); !isErr || !strings.Contains(out, "verbose") {
		t.Fatalf("unknown level = %s", out)
	}
}

func TestCutsKeepUTF8Valid(t *testing.T) {
	t.Parallel()
	res, _, err := result(strings.Repeat("é", maxResultBytes))
	if err != nil {
		t.Fatal(err)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !utf8.ValidString(text) {
		t.Fatal("result cut inside a character")
	}
	if body := cut(strings.Repeat("é", bodyLimit)); !utf8.ValidString(body) {
		t.Fatal("body cut inside a character")
	}
}

func TestAnAmbiguousAppIDIsExplained(t *testing.T) {
	t.Parallel()
	lens := &fakeLens{devices: []queries.Device{device("dev.app", "d1", "queries"), device("dev.app", "d2", "queries")}}
	out, isErr := call(t, connect(t, lens), "list_queries", map[string]any{"device": "dev.app"})
	if !isErr || !strings.Contains(out, `"dev.app" matches several devices`) {
		t.Fatalf("ambiguous app id = %s", out)
	}
}

func TestTheBearerSchemeIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	ep, err := Start(oneDevice(), 0, Options{Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ep.Close() })
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lowercase bearer: %d", resp.StatusCode)
	}
}

func TestCloseDoesNotWaitForConnectedClients(t *testing.T) {
	t.Parallel()
	ep, err := Start(oneDevice(), 0, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ep.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	mustCall(t, cs, "status", nil)
	start := time.Now()
	_ = ep.Close()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Close took %v with a client connected", d)
	}
}

func TestHints(t *testing.T) {
	t.Parallel()
	res, err := connect(t, oneDevice()).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		a := tool.Annotations
		if a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: tools act on the attached app only (openWorldHint must be false)", tool.Name)
		}
		if tool.Name == "reload_app" && (a.DestructiveHint == nil || !*a.DestructiveHint) {
			t.Errorf("reload_app loses the app's in-memory state and must be destructive")
		}
	}
}

// The DNS-rebinding check needs a real listener: the SDK only applies it to
// requests that arrived on a local address.
func TestARealListenerRefusesForeignHosts(t *testing.T) {
	t.Parallel()
	ep, err := Start(oneDevice(), 0, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ep.Close() })
	post := func(host string) int {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("evil.example"); code != http.StatusForbidden {
		t.Fatalf("Host evil.example: %d, want 403", code)
	}
	if code := post("localhost"); code != http.StatusOK {
		t.Fatalf("Host localhost: %d, want 200", code)
	}
}

func TestQueryActionsNeedAQuery(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	cs := connect(t, lens)
	if out, isErr := call(t, cs, "query_action", map[string]any{"action": "REFETCH"}); !isErr || !strings.Contains(out, "needs queryHash") {
		t.Fatalf("REFETCH without a query = %s", out)
	}
	mustCall(t, cs, "query_action", map[string]any{"action": "CLEAR_QUERY_CACHE"})
	if got := lens.Calls(); len(got) != 1 || !strings.Contains(got[0], "CLEAR_QUERY_CACHE") {
		t.Fatalf("calls = %v", got)
	}
}

// Not parallel: it shortens evaluateTimeout for its duration.
func TestEvaluateGivesUp(t *testing.T) {
	lens := &slowLens{fakeLens: oneDevice()}
	old := evaluateTimeout
	t.Cleanup(func() { evaluateTimeout = old })
	evaluateTimeout = 50 * time.Millisecond
	out, isErr := call(t, connect(t, lens), "evaluate_js", map[string]any{"expression": "new Promise(() => {})"})
	if !isErr || !strings.Contains(out, "did not finish") {
		t.Fatalf("hanging evaluate = %s", out)
	}
}

// slowLens never finishes an evaluation until its context ends.
type slowLens struct{ *fakeLens }

func (s *slowLens) Evaluate(ctx context.Context, _ string) (console.Entry, error) {
	<-ctx.Done()
	return console.Entry{}, ctx.Err()
}

// With several devices, the one running the attached app is the default.
func TestTheAttachedAppsDeviceIsTheDefault(t *testing.T) {
	t.Parallel()
	attached := device("Lens Example", "be61", "queries", "storage")
	attached.App.Platform, attached.App.RNVersion, attached.Device.Name = "ios", "0.88.1", "iPhone 17 Pro"
	expoGo := device("Lens Example", "7611", "queries")
	expoGo.App.Platform, expoGo.App.RNVersion, expoGo.Device.Name = "ios", "0.88.0", "iPhone 15"
	lens := &fakeLens{devices: []queries.Device{expoGo, attached}, session: attachedTo("iPhone 17 Pro", "ios", "0.88.1")}
	cs := connect(t, lens)
	mustCall(t, cs, "list_queries", nil)
	if got := lens.Calls(); len(got) != 1 || got[0] != "queries:Lens Example|be61" {
		t.Fatalf("calls = %v", got)
	}
	// No attached app that matches: still ambiguous, and the error says why.
	lens.session = attachedTo("Pixel 9", "android", "0.88.1")
	if out, isErr := call(t, cs, "list_queries", nil); !isErr || !strings.Contains(out, "several devices") {
		t.Fatalf("no match = %s", out)
	}
}

func TestStatusShowsEachDevicesConnection(t *testing.T) {
	t.Parallel()
	connected := time.Now().Add(-time.Hour)
	d := device("Lens Example", "be61", "queries")
	d.App.Platform, d.App.RNVersion, d.Device.Name = "ios", "0.88.1", "iPhone 17 Pro"
	d.ConnectedAt, d.LastMessageAt = connected, time.Now().Add(-90*time.Second)
	lens := &fakeLens{devices: []queries.Device{d}, session: attachedTo("iPhone 17 Pro", "ios", "0.88.1")}
	out := mustCall(t, connect(t, lens), "status", nil)
	var st struct {
		SDKDevices []struct {
			Key           string    `json:"key"`
			ConnectedAt   time.Time `json:"connectedAt"`
			LastMessageAt time.Time `json:"lastMessageAt"`
			IdleSeconds   int       `json:"idleSeconds"`
			AttachedApp   bool      `json:"attachedApp"`
		} `json:"sdkDevices"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil || len(st.SDKDevices) != 1 {
		t.Fatalf("status = %s (%v)", out, err)
	}
	got := st.SDKDevices[0]
	if !got.ConnectedAt.Equal(connected.Truncate(time.Second)) && got.ConnectedAt.IsZero() {
		t.Fatalf("connectedAt = %v", got.ConnectedAt)
	}
	if got.IdleSeconds < 89 || got.IdleSeconds > 95 || !got.AttachedApp {
		t.Fatalf("device = %+v", got)
	}
}

// Fields the SDK sent as null and fields it left out read the same.
func TestNullErrorFieldsAreDropped(t *testing.T) {
	t.Parallel()
	lens := oneDevice()
	lens.queries = []queries.Query{{Query: protocol.Query{QueryHash: "q", QueryKey: json.RawMessage(`["q"]`), State: protocol.QueryState{Status: "success", Error: json.RawMessage("null"), Data: json.RawMessage(`{"a":null}`)}}}}
	lens.mutations = []protocol.Mutation{{MutationID: 1, State: protocol.MutationState{Status: "success", Error: json.RawMessage("null"), FailureReason: json.RawMessage("null"), Data: json.RawMessage("null")}}}
	cs := connect(t, lens)
	for tool, args := range map[string]map[string]any{"list_queries": nil, "get_query": {"queryHash": "q"}, "list_mutations": nil} {
		out := mustCall(t, cs, tool, args)
		if strings.Contains(out, `"error": null`) || strings.Contains(out, `"failureReason": null`) {
			t.Fatalf("%s kept null error fields: %s", tool, out)
		}
	}
	if out := mustCall(t, cs, "get_query", map[string]any{"queryHash": "q"}); !strings.Contains(out, `"a": null`) {
		t.Fatalf("null inside data must stay: %s", out)
	}
	if out := mustCall(t, cs, "list_mutations", nil); !strings.Contains(out, `"data": null`) {
		t.Fatalf("a mutation's null result is data and must stay: %s", out)
	}
}

// On iOS the SDK reports the device name as "iOS", so the app's identity
// (its name against the attached app's display name) and the React Native
// version have to do the matching. These are the values a real attach
// reported.
func TestTheAttachedAppMatchesByIdentityWhenDeviceNamesDiffer(t *testing.T) {
	t.Parallel()
	devApp := device("Lens Example", "18f5", "queries")
	devApp.App.Name, devApp.App.Platform, devApp.App.RNVersion, devApp.Device.Name = "Lens Example", "ios", "0.88.1", "iOS"
	expoGo := device("Lens Example", "7611", "queries")
	expoGo.App.Name, expoGo.App.Platform, expoGo.App.RNVersion, expoGo.Device.Name = "Lens Example", "ios", "0.88.0", "iOS"
	other := device("Other App", "aa", "queries")
	other.App.Name, other.App.Platform, other.App.RNVersion, other.Device.Name = "Other App", "ios", "0.88.1", "iOS"
	session := attachedTo("iPhone Duo", "ios", "0.88.1")
	session.Metadata.AppDisplayName, session.Metadata.AppIdentifier = "LensExample", "dev.rnlens.example"
	lens := &fakeLens{devices: []queries.Device{other, expoGo, devApp}, session: session}
	cs := connect(t, lens)
	mustCall(t, cs, "list_queries", nil)
	if got := lens.Calls(); len(got) != 1 || got[0] != "queries:Lens Example|18f5" {
		t.Fatalf("calls = %v", got)
	}
}

// Every app on an Android device reports the same device name, so the name
// alone must never pick another app's device as the default.
func TestAnotherAppOnTheSameDeviceIsNotTheDefault(t *testing.T) {
	t.Parallel()
	sameDevice := device("com.other.b", "e1", "queries", "storage")
	sameDevice.App.Name, sameDevice.App.Platform, sameDevice.App.RNVersion, sameDevice.Device.Name = "Other B", "android", "0.88.1", "sdk_gphone64_arm64"
	pixel := device("com.other.c", "p9", "queries", "storage")
	pixel.App.Name, pixel.App.Platform, pixel.App.RNVersion, pixel.Device.Name = "Other C", "android", "0.88.1", "Pixel 9"
	session := attachedTo("sdk_gphone64_arm64", "android", "0.88.1")
	session.Metadata.AppIdentifier, session.Metadata.AppDisplayName = "com.mine.app", "Mine"
	lens := &fakeLens{devices: []queries.Device{sameDevice, pixel}, session: session}
	out, isErr := call(t, connect(t, lens), "clear_storage", map[string]any{"store": "async"})
	if !isErr || !strings.Contains(out, "several devices") {
		t.Fatalf("clear_storage defaulted to another app: %s", out)
	}
	if got := lens.Calls(); len(got) != 0 {
		t.Fatalf("calls = %v", got)
	}
}

// Write results say which device they changed.
func TestWriteResultsNameTheDevice(t *testing.T) {
	t.Parallel()
	out := mustCall(t, connect(t, oneDevice()), "clear_storage", map[string]any{"store": "async-storage"})
	if !strings.Contains(out, "dev.rnlens.example|d1") {
		t.Fatalf("result = %q", out)
	}
}
