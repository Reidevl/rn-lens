package wailsui

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
)

func TestAppServiceReportsTheMCPEndpoint(t *testing.T) {
	t.Parallel()
	svc := NewAppService(app.New(app.Config{SDKAddr: "off"}))
	if got := svc.MCP(); got != (MCPInfo{}) {
		t.Fatalf("MCP without an endpoint = %+v", got)
	}
	svc.mcp = MCPInfo{URL: "http://127.0.0.1:7431/mcp", Token: "s3cret"}
	if got := svc.MCP(); got.URL != "http://127.0.0.1:7431/mcp" || got.Token != "s3cret" {
		t.Fatalf("MCP = %+v", got)
	}
}
