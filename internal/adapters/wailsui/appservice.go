package wailsui

import "github.com/Reidevl/rn-lens/internal/app"

// AppService exposes build information to the frontend.
type AppService struct {
	core      *app.App
	transfers *transfers // nil in the desktop build
	mcp       MCPInfo
}

// MCPInfo says where AI clients reach Lens over MCP; empty when it is off.
type MCPInfo struct {
	URL string `json:"url"`
	// Token is the bearer token clients must send, when one is set.
	Token string `json:"token,omitempty"`
}

// NewAppService wraps core for the Wails bindings generator.
func NewAppService(core *app.App) *AppService {
	return &AppService{core: core}
}

// Info returns the name and version of this Lens build.
func (s *AppService) Info() app.Info {
	return s.core.Info()
}

// MCP returns the MCP endpoint, for the status bar's connect command.
func (s *AppService) MCP() MCPInfo { return s.mcp }

// DownloadURL returns the one-time browser download URL of a file a save
// wrote in browser mode, or "" when there is none (desktop mode, where the
// file is already where the user chose).
func (s *AppService) DownloadURL(path string) string {
	if s.transfers == nil {
		return ""
	}
	return s.transfers.downloadURL(path)
}
