package main

import (
	"fmt"

	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
	"github.com/Reidevl/rn-lens/internal/app"
)

// mcpTokenEnv holds the MCP token. Other local users can read a process's
// command line (ps, /proc), so the environment is the safe place for it.
const mcpTokenEnv = "RNLENS_MCP_TOKEN" //nolint:gosec // the variable's name, not a credential

// mcpToken picks the MCP token: the environment first, then the flag, with
// a warning when it came from the command line.
func mcpToken(flagValue, envValue string) (token, warning string) {
	if envValue != "" {
		return envValue, ""
	}
	if flagValue != "" {
		return flagValue, "warning: --mcp-token is visible to other users of this machine (ps); set " + mcpTokenEnv + " instead"
	}
	return "", ""
}

// startMCP starts the MCP endpoint, or reports why it is off. Lens runs
// either way: port 0 turns MCP off, a busy port (another Lens) disables it.
func startMCP(core *app.App, port int, token string) (*mcpserver.Endpoint, string) {
	if port == 0 {
		return nil, ""
	}
	ep, err := mcpserver.Start(mcpserver.AppLens{App: core}, port, mcpserver.Options{Token: token, Version: buildVersion()})
	if err != nil {
		return nil, fmt.Sprintf("rnlens: MCP is off: %v", err)
	}
	return ep, ""
}
