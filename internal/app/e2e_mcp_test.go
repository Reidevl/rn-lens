package app_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
	"github.com/Reidevl/rn-lens/internal/app"
)

// TestE2E_MCPManagesTheApp drives the example app (LENS_E2E_METRO and
// LENS_E2E_SDK_ADDR, as in TestE2E_StorageEditReachesTheApp) through a real
// MCP client over HTTP: it attaches, reads queries, storage and plugins,
// and changes them.
func TestE2E_MCPManagesTheApp(t *testing.T) {
	endpoint, sdkAddr := os.Getenv("LENS_E2E_METRO"), os.Getenv("LENS_E2E_SDK_ADDR")
	if endpoint == "" || sdkAddr == "" || testing.Short() {
		t.Skip("set LENS_E2E_METRO and LENS_E2E_SDK_ADDR with the example app running to enable")
	}
	a := app.New(app.Config{MetroEndpoints: []string{endpoint}, PollInterval: 200 * time.Millisecond, SDKAddr: sdkAddr})
	a.Start()
	t.Cleanup(a.Stop)
	ep, err := mcpserver.Start(mcpserver.AppLens{App: a}, 0, mcpserver.Options{Version: "e2e"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ep.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ep.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	tool := func(name string, args map[string]any) (string, bool) {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String(), res.IsError
	}
	must := func(name string, args map[string]any) string {
		t.Helper()
		out, isErr := tool(name, args)
		if isErr {
			t.Fatalf("%s failed: %s", name, out)
		}
		return out
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for !ok() {
			select {
			case <-ctx.Done():
				t.Fatalf("timed out waiting for %s", what)
			case <-time.After(250 * time.Millisecond):
			}
		}
	}

	var list []struct {
		ID    string `json:"id"`
		AppID string `json:"appId"`
	}
	target := ""
	waitFor("the example app's target", func() bool {
		_ = json.Unmarshal([]byte(must("list_targets", nil)), &list)
		for _, tg := range list {
			if tg.AppID == "dev.rnlens.example" {
				target = tg.ID
			}
		}
		return target != ""
	})
	must("attach_target", map[string]any{"targetId": target})
	waitFor("the attach", func() bool { return strings.Contains(must("status", nil), `"state": "attached"`) })
	if out := must("evaluate_js", map[string]any{"expression": "6 * 7"}); !strings.Contains(out, `"value": 42`) {
		t.Fatalf("evaluate_js = %s", out)
	}

	// The SDK connects to this Lens's SDK server on its own.
	waitFor("the SDK device", func() bool {
		out, isErr := tool("list_queries", nil)
		return !isErr && strings.Contains(out, "queryHash")
	})
	// The SDK device running the attached app is recognised as such.
	if out := must("status", nil); !strings.Contains(out, `"attachedApp": true`) {
		t.Fatalf("no SDK device matched the attached app: %s", out[:min(len(out), 6000)])
	}
	var page struct {
		Queries []struct {
			QueryHash string `json:"queryHash"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(must("list_queries", nil)), &page); err != nil || len(page.Queries) == 0 {
		t.Fatalf("queries = %+v (%v)", page, err)
	}
	must("query_action", map[string]any{"action": "INVALIDATE", "queryHash": page.Queries[0].QueryHash})
	t.Logf("invalidated %s", page.Queries[0].QueryHash)

	must("set_storage_value", map[string]any{"store": "async-storage", "key": "lens-e2e-mcp", "value": "from MCP"})
	if out := must("get_storage_value", map[string]any{"store": "async-storage", "key": "lens-e2e-mcp"}); !strings.Contains(out, "from MCP") {
		t.Fatalf("value = %s", out)
	}
	must("remove_storage_key", map[string]any{"store": "async-storage", "key": "lens-e2e-mcp"})
	if out, isErr := tool("get_storage_value", map[string]any{"store": "async-storage", "key": "lens-e2e-mcp"}); !isErr {
		t.Fatalf("the key is still there: %s", out)
	}

	if out := must("list_plugins", nil); !strings.Contains(out, "feature-flags") {
		t.Fatalf("plugins = %s", out)
	}
	msg := must("plugin_action", map[string]any{"plugin": "feature-flags", "block": "flags", "action": "toggle", "row": "darkMode"})
	t.Logf("plugin answered %q", msg)
	if !strings.Contains(msg, "darkMode is") {
		t.Fatalf("plugin answer = %q", msg)
	}
	must("plugin_action", map[string]any{"plugin": "feature-flags", "block": "flags", "action": "toggle", "row": "darkMode"}) // back

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "take_screenshot"})
	if err != nil || res.IsError || len(res.Content) != 1 {
		t.Fatalf("screenshot = %+v, %v", res, err)
	}
	if img, ok := res.Content[0].(*mcp.ImageContent); !ok || !strings.HasPrefix(string(img.Data), "\x89PNG") {
		t.Fatalf("screenshot is not a PNG")
	}
}
