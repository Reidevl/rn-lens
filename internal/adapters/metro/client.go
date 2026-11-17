// Package metro talks to Metro's inspector proxy over HTTP: it lists debug
// targets (/json), reads the protocol version (/json/version) and asks Metro
// to open React Native DevTools (/open-debugger).
//
// The endpoint shapes come from @react-native/dev-middleware
// (packages/dev-middleware/src/inspector-proxy).
package metro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Capabilities are the target capability flags Metro reports under
// reactNative.capabilities.
type Capabilities struct {
	NativePageReloads         bool `json:"nativePageReloads"`
	NativeSourceCodeFetching  bool `json:"nativeSourceCodeFetching"`
	SupportsMultipleDebuggers bool `json:"supportsMultipleDebuggers"`
}

// ReactNativeInfo is the reactNative block of a page description.
type ReactNativeInfo struct {
	LogicalDeviceID string       `json:"logicalDeviceId"`
	Capabilities    Capabilities `json:"capabilities"`
}

// Page is one entry of GET /json (a "PageDescription").
type Page struct {
	ID                   string          `json:"id"`
	Title                string          `json:"title"`
	AppID                string          `json:"appId"`
	Description          string          `json:"description"`
	Type                 string          `json:"type"`
	DevtoolsFrontendURL  string          `json:"devtoolsFrontendUrl"`
	WebSocketDebuggerURL string          `json:"webSocketDebuggerUrl"`
	VM                   string          `json:"vm,omitempty"`
	DeviceName           string          `json:"deviceName"`
	ReactNative          ReactNativeInfo `json:"reactNative"`
}

// PageID returns the device-local page id, the part after the logical
// device id in the proxy's "<deviceId>-<pageId>" composite.
func (p Page) PageID() string {
	prefix := p.ReactNative.LogicalDeviceID + "-"
	if strings.HasPrefix(p.ID, prefix) {
		return strings.TrimPrefix(p.ID, prefix)
	}
	return p.ID
}

// IsReloadable reports whether the page is the modern (Fusebox) target that
// survives JS reloads. Metro's own /open-debugger selects targets this way.
func (p Page) IsReloadable() bool {
	return p.ReactNative.Capabilities.NativePageReloads ||
		p.Title == "React Native Experimental (Improved Chrome Reloads)"
}

// Version is the reply of GET /json/version.
type Version struct {
	Browser         string `json:"Browser"`
	ProtocolVersion string `json:"Protocol-Version"`
}

// Client issues requests against one or more Metro base URLs.
type Client struct {
	http *http.Client
}

// NewClient returns a client. A nil httpClient uses a 3 second timeout.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{http: httpClient}
}

// ListPages fetches GET <baseURL>/json.
func (c *Client) ListPages(ctx context.Context, baseURL string) ([]Page, error) {
	var pages []Page
	if err := c.getJSON(ctx, baseURL+"/json", &pages); err != nil {
		return nil, err
	}
	return pages, nil
}

// Version fetches GET <baseURL>/json/version.
func (c *Client) Version(ctx context.Context, baseURL string) (Version, error) {
	var v Version
	err := c.getJSON(ctx, baseURL+"/json/version", &v)
	return v, err
}

// OpenDebugger asks Metro to launch React Native DevTools for pageID
// (POST /open-debugger?target=<pageID>). Metro expects the composite id from
// /json, not the device-local one.
func (c *Client) OpenDebugger(ctx context.Context, baseURL, pageID string) error {
	target := baseURL + "/open-debugger?target=" + url.QueryEscape(pageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("metro: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("metro: open-debugger: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("metro: open-debugger returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("metro: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("metro: get %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metro: get %s returned %d", target, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("metro: decode %s: %w", target, err)
	}
	return nil
}
