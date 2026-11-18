package wailsui

import (
	"testing"

	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
	"github.com/Reidevl/rn-lens/internal/app"
)

func TestMCPServiceWhenMCPIsOff(t *testing.T) {
	t.Parallel()
	svc := NewMCPService(MCPInfo{}, nil)
	if st := svc.Status(); st.Enabled || st.URL != "" || st.ReadOnly {
		t.Fatalf("Status = %+v", st)
	}
	if tools := svc.Tools(); tools == nil || len(tools) != 0 {
		t.Fatalf("Tools = %#v", tools)
	}
	if calls := svc.Activity(0); calls == nil || len(calls) != 0 {
		t.Fatalf("Activity = %#v", calls)
	}
	if err := svc.SetReadOnly(true); err == nil {
		t.Fatal("SetReadOnly without MCP succeeded")
	}
	svc.ClearActivity() // no panic
}

func TestMCPServiceReadsAndSwitchesTheControl(t *testing.T) {
	t.Parallel()
	core := app.New(app.Config{SDKAddr: "off", MetroEndpoints: []string{"http://127.0.0.1:1"}})
	ep, err := mcpserver.Start(mcpserver.AppLens{App: core}, 0, mcpserver.Options{Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ep.Close() })
	svc := NewMCPService(MCPInfo{URL: ep.URL, Token: "t"}, ep.Control)
	if st := svc.Status(); !st.Enabled || st.URL != ep.URL || !st.TokenSet || st.ReadOnly {
		t.Fatalf("Status = %+v", st)
	}
	if len(svc.Tools()) != 21 {
		t.Fatalf("Tools = %d", len(svc.Tools()))
	}
	if err := svc.SetReadOnly(true); err != nil || !svc.Status().ReadOnly || !ep.Control.ReadOnly() {
		t.Fatalf("SetReadOnly: %v", err)
	}
}
