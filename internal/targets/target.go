// Package targets owns everything about which React Native app Lens is
// looking at: the targets Metro advertises, the attach session with its
// reconnect rules, and the app metadata a Fusebox-aware target reports.
package targets

import (
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
)

// Capabilities are the target capability flags relevant to Lens.
type Capabilities struct {
	// NativePageReloads: the page survives JS reloads (Fusebox target).
	NativePageReloads bool `json:"nativePageReloads"`
	// NativeSourceCodeFetching: the app fetches sources itself; the proxy does not rewrite URLs.
	NativeSourceCodeFetching bool `json:"nativeSourceCodeFetching"`
	// SupportsMultipleDebuggers: Lens can attach next to React Native DevTools.
	// When false, attaching evicts the other debugger ("exclusive attach").
	SupportsMultipleDebuggers bool `json:"supportsMultipleDebuggers"`
}

// Target is a debuggable page advertised by a Metro endpoint.
type Target struct {
	// ID is unique across endpoints: "<endpoint>|<metro page id>".
	ID string `json:"id"`
	// Endpoint is the Metro base URL that listed the page.
	Endpoint string `json:"endpoint"`
	// MetroID is Metro's composite "<deviceId>-<pageId>", used by /open-debugger.
	MetroID string `json:"metroId"`
	// PageID is the device-local page id.
	PageID          string       `json:"pageId"`
	Title           string       `json:"title"`
	AppID           string       `json:"appId"`
	DeviceName      string       `json:"deviceName"`
	LogicalDeviceID string       `json:"logicalDeviceId"`
	WebSocketURL    string       `json:"webSocketUrl"`
	VM              string       `json:"vm,omitempty"`
	Capabilities    Capabilities `json:"capabilities"`
	// Reloadable marks the modern target that survives reloads; Lens attaches
	// to these by default, the same way Metro's /open-debugger picks them.
	Reloadable bool `json:"reloadable"`
}

// FromMetro converts a discovered Metro page into a Target.
func FromMetro(d metro.Discovered) Target {
	caps := d.Page.ReactNative.Capabilities
	return Target{
		ID:              d.Endpoint + "|" + d.Page.ID,
		Endpoint:        d.Endpoint,
		MetroID:         d.Page.ID,
		PageID:          d.Page.PageID(),
		Title:           d.Page.Title,
		AppID:           d.Page.AppID,
		DeviceName:      d.Page.DeviceName,
		LogicalDeviceID: d.Page.ReactNative.LogicalDeviceID,
		WebSocketURL:    d.Page.WebSocketDebuggerURL,
		VM:              d.Page.VM,
		Capabilities: Capabilities{
			NativePageReloads:         caps.NativePageReloads,
			NativeSourceCodeFetching:  caps.NativeSourceCodeFetching,
			SupportsMultipleDebuggers: caps.SupportsMultipleDebuggers,
		},
		Reloadable: d.Page.IsReloadable(),
	}
}

// Metadata is what a Fusebox target reports in
// ReactNativeApplication.metadataUpdated.
type Metadata struct {
	AppDisplayName           string `json:"appDisplayName"`
	AppIdentifier            string `json:"appIdentifier"`
	DeviceName               string `json:"deviceName"`
	IntegrationName          string `json:"integrationName"`
	Platform                 string `json:"platform"`
	ReactNativeVersion       string `json:"reactNativeVersion"`
	IsProfilingBuild         bool   `json:"unstable_isProfilingBuild"`
	NetworkInspectionEnabled bool   `json:"unstable_networkInspectionEnabled"`
	FrameRecordingEnabled    bool   `json:"unstable_frameRecordingEnabled"`
	NetworkThrottlingEnabled bool   `json:"unstable_networkThrottlingEnabled"`
}

// SameApp reports whether other represents the same application instance on
// the same device as t, ignoring the page id, which changes across restarts.
func (t Target) SameApp(other Target) bool {
	if t.Endpoint != other.Endpoint || t.AppID != other.AppID {
		return false
	}
	if t.LogicalDeviceID != "" && other.LogicalDeviceID != "" {
		return t.LogicalDeviceID == other.LogicalDeviceID
	}
	return t.DeviceName == other.DeviceName
}

// FindReplacement picks, from candidates, the reloadable page that now stands
// for the same app and device as previous. It returns false when none exists.
func FindReplacement(previous Target, candidates []Target) (Target, bool) {
	var fallback Target
	found := false
	for _, c := range candidates {
		if c.ID == previous.ID || !previous.SameApp(c) {
			continue
		}
		if c.Reloadable {
			return c, true
		}
		if !found {
			fallback, found = c, true
		}
	}
	return fallback, found
}
