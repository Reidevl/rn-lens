package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Reidevl/rn-lens/internal/queries"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// DefaultPort is where Lens serves MCP; 7420 is the SDK's, 7430 browser mode's.
const DefaultPort = 7431

// maxResultBytes bounds the text of one tool result so a large cache or body
// does not flood the AI's context.
const maxResultBytes = 256 << 10

// Options configures the MCP endpoint.
type Options struct {
	// Token, when set, must be sent as "Authorization: Bearer <token>".
	Token string
	// Version is reported to clients as the server version.
	Version string
	// ReadOnly starts with the tools that change the app refused.
	ReadOnly bool
	// OnChange runs after the activity log or the read-only switch
	// changed. It must not block.
	OnChange func()
}

// NewServer returns the MCP server with every Lens tool, and the Control
// that records its calls and switches read-only mode.
func NewServer(lens Lens, opts Options) (*mcp.Server, *Control) {
	s := mcp.NewServer(&mcp.Implementation{Name: "lens", Title: "Lens — React Native DevTools", Version: opts.Version}, &mcp.ServerOptions{
		Instructions: "Lens is attached to a React Native app. Start with `status` to see the attached app " +
			"and the devices running @rnlens/sdk, and `list_targets` for the apps Metro lists. Device " +
			"arguments are optional when one device is connected. Tools that change the app are marked destructive.",
	})
	ctl := newControl(opts)
	addReadTools(s, ctl, lens)
	addWriteTools(s, ctl, lens)
	s.AddReceivingMiddleware(ctl.middleware)
	return s, ctl
}

// Handler serves MCP over streamable HTTP at any path. It refuses
// cross-origin browser requests and Host headers other than localhost (DNS
// rebinding), and checks the token when one is set.
func Handler(server *mcp.Server, opts Options) http.Handler {
	h := http.NewCrossOriginProtection().Handler(
		mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
			// Free the sessions of clients that went away without closing.
			SessionTimeout: 30 * time.Minute,
		}))
	if opts.Token == "" {
		return h
	}
	want := []byte(opts.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		// The scheme is case-insensitive (RFC 7235); the token is not.
		if !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare([]byte(token), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="lens"`)
			http.Error(w, "lens: missing or wrong token", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Endpoint is a running MCP endpoint.
type Endpoint struct {
	URL string
	// Control is the endpoint's activity log, read-only switch and tools.
	Control *Control
	srv     *http.Server
}

// Start listens on 127.0.0.1:port (0 picks a free port) and serves MCP at
// /mcp until Close.
func Start(lens Lens, port int, opts Options) (*Endpoint, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("mcp: listen on port %d: %w", port, err)
	}
	server, ctl := NewServer(lens, opts)
	mux := http.NewServeMux()
	mux.Handle("/mcp", Handler(server, opts))
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return &Endpoint{URL: fmt.Sprintf("http://%s/mcp", ln.Addr()), Control: ctl, srv: srv}, nil
}

// Close stops the endpoint, ending open sessions. A connected client keeps a
// stream open, so the graceful wait is short before connections are cut.
func (e *Endpoint) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := e.srv.Shutdown(ctx); err != nil {
		return e.srv.Close()
	}
	return nil
}

// result renders v as indented JSON text, cut at maxResultBytes.
func result(v any) (*mcp.CallToolResult, any, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	text := string(raw)
	if len(text) > maxResultBytes {
		text = truncateUTF8(text, maxResultBytes) + "\n… (cut at 256 KiB, so this is not valid JSON; ask for less: search or limit where the tool has them)"
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// truncateUTF8 cuts s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// message renders a short confirmation.
func message(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}, nil, nil
}

// errNoDevice explains how to connect a device.
var errNoDevice = errors.New("no app with @rnlens/sdk is connected for this feature; add useLens(...) to the app (see `status`)")

// choose resolves the optional device argument for lens's attached app.
func choose(lens Lens, arg string, keys []string) (string, error) {
	return pickDevice(arg, keys, attachedKey(lens))
}

// pickDevice resolves the optional device argument among keys: the key
// itself, an app id that matches exactly one device, the only device, or
// preferred (the device running the attached app) when several qualify.
func pickDevice(arg string, keys []string, preferred string) (string, error) {
	if len(keys) == 0 {
		return "", errNoDevice
	}
	if arg == "" {
		if len(keys) == 1 {
			return keys[0], nil
		}
		if preferred != "" && slices.Contains(keys, preferred) {
			return preferred, nil
		}
		return "", fmt.Errorf("several devices are connected and none is the attached app's; pass device as one of: %s", strings.Join(keys, ", "))
	}
	var byApp []string
	for _, k := range keys {
		if k == arg {
			return k, nil
		}
		if app, _, _ := strings.Cut(k, "|"); app == arg {
			byApp = append(byApp, k)
		}
	}
	if len(byApp) == 1 {
		return byApp[0], nil
	}
	if len(byApp) > 1 {
		return "", fmt.Errorf("app id %q matches several devices; pass one of: %s", arg, strings.Join(byApp, ", "))
	}
	return "", fmt.Errorf("unknown device %q; connected: %s", arg, strings.Join(keys, ", "))
}

// attachedKey returns the SDK device running the app Lens is attached to.
// Its platform and React Native version must agree with the attached app's
// (when both sides report them). When the attached app reports its bundle
// id or display name, the device's app must match it (its id against the
// bundle id, or its id or name against the display name, spaces and case
// ignored): every app on an Android device reports the same device name,
// so the name alone could pick another app. The device name then only
// breaks a tie, or decides when the attached app reports no identity.
// "" when nothing matches exactly one device.
func attachedKey(lens Lens) string {
	snap := lens.Session()
	md := snap.Metadata
	if snap.State != targets.StateAttached || md == nil {
		return ""
	}
	same := func(a, b string) bool { return a == "" || b == "" || strings.EqualFold(a, b) }
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "")) }
	nameOf := func(d queries.Device) bool {
		return d.Device.Name != "" && strings.EqualFold(d.Device.Name, md.DeviceName)
	}
	appOf := func(d queries.Device) bool {
		display := norm(md.AppDisplayName)
		return (md.AppIdentifier != "" && d.App.ID == md.AppIdentifier) ||
			(display != "" && (norm(d.App.ID) == display || norm(d.App.Name) == display))
	}
	identity := md.AppIdentifier != "" || md.AppDisplayName != ""
	var matches, byName []queries.Device
	for _, d := range lens.SDKDevices() {
		if !same(d.App.Platform, md.Platform) || !same(d.App.RNVersion, md.ReactNativeVersion) {
			continue
		}
		if identity && !appOf(d) || !identity && !nameOf(d) {
			continue
		}
		matches = append(matches, d)
		if nameOf(d) {
			byName = append(byName, d)
		}
	}
	if len(matches) > 1 && len(byName) == 1 {
		matches = byName
	}
	if len(matches) != 1 {
		return ""
	}
	return matches[0].Key
}

// dropNull treats a JSON null like an absent field.
func dropNull(raw json.RawMessage) json.RawMessage {
	if string(raw) == "null" {
		return nil
	}
	return raw
}

func ptr[T any](v T) *T { return &v }

// readOnly, changes and destructive annotate tools for the client's
// permission UI. Every tool acts on the attached app only (closed world).
var (
	readOnly    = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)}
	changes     = &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}
	destructive = &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)}
)
