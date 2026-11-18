package mcpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/console"
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
