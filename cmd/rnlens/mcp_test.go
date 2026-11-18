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
	if ep, msg := startMCP(core, 0, "", false); ep != nil || msg != "" {
		t.Fatalf("--mcp-port 0 = %v, %q", ep, msg)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	busy := ln.Addr().(*net.TCPAddr).Port
	if ep, msg := startMCP(core, busy, "", false); ep != nil || !strings.Contains(msg, "MCP is off") {
		t.Fatalf("busy port = %v, %q", ep, msg)
	}
}

func TestMCPReadOnlyFlagAndNotices(t *testing.T) {
	core := app.New(app.Config{SDKAddr: "off", MetroEndpoints: []string{"http://127.0.0.1:1"}})
	ch, cancel := core.Bus().Subscribe(app.TopicMCPChanged, 4)
	defer cancel()
	ep, msg := startMCP(core, freePort(t), "", true)
	if ep == nil {
		t.Fatalf("startMCP: %s", msg)
	}
	defer func() { _ = ep.Close() }()
	if !ep.Control.ReadOnly() {
		t.Fatal("--mcp-read-only did not start read-only")
	}
	ep.Control.SetReadOnly(false)
	select {
	case <-ch:
	default:
		t.Fatal("no mcp:changed after switching read-only")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
