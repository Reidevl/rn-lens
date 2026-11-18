package main

import (
	"net"
	"strings"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
)

func TestMCPTokenPrefersTheEnvironment(t *testing.T) {
	if tok, warn := mcpToken("", "from-env"); tok != "from-env" || warn != "" {
		t.Fatalf("env token = %q, %q", tok, warn)
	}
	tok, warn := mcpToken("from-flag", "")
	if tok != "from-flag" || !strings.Contains(warn, "RNLENS_MCP_TOKEN") || !strings.Contains(warn, "ps") {
		t.Fatalf("flag token = %q, warning %q", tok, warn)
	}
	if tok, _ := mcpToken("from-flag", "from-env"); tok != "from-env" {
		t.Fatalf("the environment must win, got %q", tok)
	}
	if tok, warn := mcpToken("", ""); tok != "" || warn != "" {
		t.Fatalf("no token = %q, %q", tok, warn)
	}
}

func TestMCPOffOrBusyKeepsLensRunning(t *testing.T) {
	core := app.New(app.Config{SDKAddr: "off", MetroEndpoints: []string{"http://127.0.0.1:1"}})
	if ep, msg := startMCP(core, 0, ""); ep != nil || msg != "" {
		t.Fatalf("--mcp-port 0 = %v, %q", ep, msg)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	busy := ln.Addr().(*net.TCPAddr).Port
	if ep, msg := startMCP(core, busy, ""); ep != nil || !strings.Contains(msg, "MCP is off") {
		t.Fatalf("busy port = %v, %q", ep, msg)
	}
}
