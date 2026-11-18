package mcpserver

import (
	"strings"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
)

// The adapter over a real (idle) Lens answers every read tool without an app
// attached, and explains what is missing instead of failing obscurely.
func TestAppLensWithoutAnApp(t *testing.T) {
	t.Parallel()
	core := app.New(app.Config{SDKAddr: "off", MetroEndpoints: []string{"http://127.0.0.1:1"}, Version: "1.2.3"})
	cs := connect(t, AppLens{App: core})
	if out := mustCall(t, cs, "status", nil); !strings.Contains(out, `"version": "1.2.3"`) || !strings.Contains(out, `"state": "idle"`) {
		t.Fatalf("status = %s", out)
	}
	for _, tool := range []string{"console_logs", "network_requests", "list_targets"} {
		if out := mustCall(t, cs, tool, nil); out != "[]" {
			t.Fatalf("%s on an idle Lens = %s", tool, out)
		}
	}
	if out, isErr := call(t, cs, "list_queries", nil); !isErr || !strings.Contains(out, "no app with @rnlens/sdk") {
		t.Fatalf("list_queries = %s", out)
	}
	if out, isErr := call(t, cs, "take_screenshot", nil); !isErr || out == "" {
		t.Fatalf("screenshot without an app = %s", out)
	}
}
