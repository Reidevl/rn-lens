package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxActivity bounds the activity log; older calls are dropped.
const maxActivity = 500

// summaryLimit bounds the result text kept per call.
const summaryLimit = 2 << 10

// ToolKind says what a tool does to the app.
type ToolKind string

// Tool kinds, from the tool's annotations.
const (
	KindRead        ToolKind = "read"
	KindChange      ToolKind = "change"
	KindDestructive ToolKind = "destructive"
)

// ToolInfo describes one tool for the MCP panel.
type ToolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kind        ToolKind `json:"kind"`
}

// Call is one tool call an AI client made.
type Call struct {
	Seq    uint64          `json:"seq"`
	Time   time.Time       `json:"time"`
	Client string          `json:"client"`
	Tool   string          `json:"tool"`
	Kind   ToolKind        `json:"kind"`
	Args   json.RawMessage `json:"args,omitempty"`
	// DurationMs is how long the tool ran.
	DurationMs float64 `json:"durationMs"`
	// Result is the start of what the tool returned.
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	// Refused means read-only mode stopped the call before it ran.
	Refused bool `json:"refused,omitempty"`
}

// Control is what the MCP panel reads and changes: the activity log, the
// read-only switch and the tool list.
type Control struct {
	readOnly atomic.Bool
	notify   func()

	mu    sync.Mutex
	calls []Call
	seq   uint64
	tools []ToolInfo
	kinds map[string]ToolKind
}

func newControl(opts Options) *Control {
	c := &Control{notify: opts.OnChange, kinds: map[string]ToolKind{}}
	if c.notify == nil {
		c.notify = func() {}
	}
	c.readOnly.Store(opts.ReadOnly)
	return c
}

// ReadOnly reports whether tools that change the app are refused.
func (c *Control) ReadOnly() bool { return c.readOnly.Load() }

// SetReadOnly turns read-only mode on or off.
func (c *Control) SetReadOnly(on bool) {
	if c.readOnly.Swap(on) != on {
		c.notify()
	}
}

// Tools lists every tool in registration order.
func (c *Control) Tools() []ToolInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ToolInfo(nil), c.tools...)
}

// Activity returns the recorded calls with Seq > after, oldest first.
func (c *Control) Activity(after uint64) []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []Call{}
	for _, call := range c.calls {
		if call.Seq > after {
			out = append(out, call)
		}
	}
	return out
}

// ClearActivity drops every recorded call; sequence numbers keep counting.
func (c *Control) ClearActivity() {
	c.mu.Lock()
	c.calls = nil
	c.mu.Unlock()
	c.notify()
}

func (c *Control) register(tool *mcp.Tool) {
	kind := KindRead
	if a := tool.Annotations; a == nil || !a.ReadOnlyHint {
		kind = KindChange
		if a != nil && a.DestructiveHint != nil && *a.DestructiveHint {
			kind = KindDestructive
		}
	}
	c.mu.Lock()
	c.tools = append(c.tools, ToolInfo{Name: tool.Name, Description: tool.Description, Kind: kind})
	c.kinds[tool.Name] = kind
	c.mu.Unlock()
}

func (c *Control) record(call Call) {
	c.mu.Lock()
	c.seq++
	call.Seq = c.seq
	c.calls = append(c.calls, call)
	if len(c.calls) > maxActivity {
		c.calls = append([]Call(nil), c.calls[len(c.calls)-maxActivity:]...)
	}
	c.mu.Unlock()
	c.notify()
}

// middleware records tools/call requests and refuses changes in read-only
// mode before the tool runs.
func (c *Control) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		callReq, ok := req.(*mcp.CallToolRequest)
		if method != "tools/call" || !ok {
			return next(ctx, method, req)
		}
		c.mu.Lock()
		kind, known := c.kinds[callReq.Params.Name]
		c.mu.Unlock()
		call := Call{
			Time:   time.Now(),
			Client: clientName(callReq),
			Tool:   callReq.Params.Name,
			Kind:   kind,
			Args:   append(json.RawMessage(nil), callReq.Params.Arguments...),
		}
		if known && kind != KindRead && c.ReadOnly() {
			msg := fmt.Sprintf("Lens is read-only for AI clients, so %s did not run; turn read-only off in Lens's MCP panel to allow changes", call.Tool)
			call.Refused, call.Error = true, msg
			c.record(call)
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil
		}
		res, err := next(ctx, method, req)
		call.DurationMs = float64(time.Since(call.Time).Microseconds()) / 1000
		if r, ok := res.(*mcp.CallToolResult); ok {
			summary := summarize(r)
			if r.IsError {
				call.Error = summary
			} else {
				call.Result = summary
			}
		}
		if err != nil {
			call.Error = err.Error()
		}
		c.record(call)
		return res, err
	}
}

func clientName(req *mcp.CallToolRequest) string {
	if req.Session == nil {
		return ""
	}
	p := req.Session.InitializeParams()
	if p == nil || p.ClientInfo == nil {
		return ""
	}
	return strings.TrimSpace(p.ClientInfo.Name + " " + p.ClientInfo.Version)
}

func summarize(r *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range r.Content {
		switch v := content.(type) {
		case *mcp.TextContent:
			b.WriteString(v.Text)
		case *mcp.ImageContent:
			fmt.Fprintf(&b, "[%s image, %d bytes]", v.MIMEType, len(v.Data))
		}
	}
	return truncateUTF8(b.String(), summaryLimit)
}

// addTool registers a tool with the server and in the panel's tool list.
func addTool[In, Out any](s *mcp.Server, c *Control, tool *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	c.register(tool)
	mcp.AddTool(s, tool, h)
}
