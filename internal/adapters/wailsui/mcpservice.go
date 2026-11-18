package wailsui

import (
	"errors"

	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
)

// MCPStatus is the header of the MCP panel.
type MCPStatus struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	// TokenSet says clients must send a bearer token; the token itself is
	// only in the copied command (AppService.MCP).
	TokenSet bool `json:"tokenSet"`
	ReadOnly bool `json:"readOnly"`
}

// MCPService exposes the MCP endpoint's activity log, read-only switch and
// tool list. The UI pulls after each app.TopicMCPChanged notice.
type MCPService struct {
	info    MCPInfo
	control *mcpserver.Control // nil when MCP is off
}

// NewMCPService wraps the endpoint's control; control is nil when MCP is off.
func NewMCPService(info MCPInfo, control *mcpserver.Control) *MCPService {
	return &MCPService{info: info, control: control}
}

var errMCPOff = errors.New("MCP is off: start Lens without --mcp-port 0")

// Status reports whether MCP runs, where, and the read-only switch.
func (s *MCPService) Status() MCPStatus {
	if s.control == nil {
		return MCPStatus{}
	}
	return MCPStatus{Enabled: true, URL: s.info.URL, TokenSet: s.info.Token != "", ReadOnly: s.control.ReadOnly()}
}

// Tools lists the tools AI clients can call.
func (s *MCPService) Tools() []mcpserver.ToolInfo {
	if s.control == nil {
		return []mcpserver.ToolInfo{}
	}
	return s.control.Tools()
}

// Activity returns the tool calls with Seq > after, oldest first.
func (s *MCPService) Activity(after uint64) []mcpserver.Call {
	if s.control == nil {
		return []mcpserver.Call{}
	}
	return s.control.Activity(after)
}

// ClearActivity empties the activity log.
func (s *MCPService) ClearActivity() {
	if s.control != nil {
		s.control.ClearActivity()
	}
}

// SetReadOnly refuses (true) or allows (false) the tools that change the app.
func (s *MCPService) SetReadOnly(on bool) error {
	if s.control == nil {
		return errMCPOff
	}
	s.control.SetReadOnly(on)
	return nil
}
