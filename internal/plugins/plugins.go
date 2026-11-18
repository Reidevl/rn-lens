// Package plugins mirrors the declarative panels an app adds to Lens through
// @rnlens/sdk and forwards the user's clicks back to the app. A panel is a
// list of blocks (protocol.PluginBlockTypes) the UI draws with Lens's own
// components; no plugin code runs in Lens. Blocks are kept as raw JSON.
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// Limits of one plugin.view (see the protocol README).
const (
	MaxViewBytes = 1 << 20
	MaxBlocks    = 100
	// MaxPlugins bounds the plugins one device registers.
	MaxPlugins = 50
)

// Sender delivers frames to one connected device.
type Sender interface {
	Send(typ, id string, payload any) error
}

// Device describes one connected SDK that announced the plugins feature.
type Device struct {
	// Key is "<app id>|<device id>", stable across reconnects.
	Key         string              `json:"key"`
	SessionID   string              `json:"sessionId"`
	App         protocol.AppInfo    `json:"app"`
	Device      protocol.DeviceInfo `json:"device"`
	SDKVersion  string              `json:"sdkVersion"`
	ConnectedAt time.Time           `json:"connectedAt"`
}

// Plugin is one registered plugin and its current panel.
type Plugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	// Blocks is the JSON array of the last valid view ("[]" before any).
	Blocks json.RawMessage `json:"blocks"`
	// Error explains why the last view was not shown.
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitzero"`
}

// Errors returned by Service.
var (
	ErrUnknownDevice = errors.New("plugins: unknown device")
	ErrUnknownPlugin = errors.New("plugins: unknown plugin")
	ErrStaleSession  = errors.New("plugins: frame from a replaced session")
	ErrInvalidPlugin = errors.New("plugins: invalid plugin")
	ErrInvalidAction = errors.New("plugins: invalid action")
	ErrActionFailed  = errors.New("plugins: action failed in the app")
	ErrDisconnected  = errors.New("plugins: device disconnected before answering")
)

var pluginID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type deviceState struct {
	info    Device
	sender  Sender
	plugins map[string]*Plugin
	order   []string // plugin ids in registration order
	pending map[string]chan protocol.PluginResult
}

// Service is the plugins feature. It is safe for concurrent use.
type Service struct {
	mu      sync.Mutex
	devices map[string]*deviceState
	notify  func()
	nextID  atomic.Uint64
	// ActionTimeout bounds an action when the context has no deadline. Set it
	// before the service is used; it is read without locking.
	ActionTimeout time.Duration
}

// NewService creates an empty service; notify runs after every change.
func NewService(notify func()) *Service {
	return &Service{devices: make(map[string]*deviceState), notify: notify, ActionTimeout: 10 * time.Second}
}

// DeviceKey builds the key used for a device (same form as the other features).
func DeviceKey(appID, deviceID string) string { return appID + "|" + deviceID }

// Connect registers a device that announced the plugins feature, replacing a
// previous one with the same key and its plugins; others are ignored.
func (s *Service) Connect(sessionID string, hello protocol.Hello, sender Sender) {
	if !slices.Contains(hello.Features, protocol.FeaturePlugins) {
		return
	}
	key := DeviceKey(hello.App.ID, hello.Device.ID)
	s.mu.Lock()
	if old, ok := s.devices[key]; ok {
		closePending(old)
	}
	s.devices[key] = &deviceState{
		info: Device{
			Key: key, SessionID: sessionID, App: hello.App, Device: hello.Device,
			SDKVersion: hello.SDKVersion, ConnectedAt: time.Now(),
		},
		sender:  sender,
		plugins: make(map[string]*Plugin),
		pending: make(map[string]chan protocol.PluginResult),
	}
	s.mu.Unlock()
	s.notify()
}

// Disconnect forgets a device if sessionID still owns it; pending actions end
// with ErrDisconnected.
func (s *Service) Disconnect(key, sessionID string) {
	s.mu.Lock()
	d, ok := s.devices[key]
	removed := ok && d.info.SessionID == sessionID
	if removed {
		closePending(d)
		delete(s.devices, key)
	}
	s.mu.Unlock()
	if removed {
		s.notify()
	}
}

func closePending(d *deviceState) {
	for id, ch := range d.pending {
		close(ch)
		delete(d.pending, id)
	}
}

// Message applies one decoded plugin.* frame from the device.
func (s *Service) Message(key, sessionID string, msg protocol.Message) error {
	s.mu.Lock()
	d, ok := s.devices[key]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownDevice
	}
	if d.info.SessionID != sessionID {
		s.mu.Unlock()
		return ErrStaleSession
	}
	changed, err := d.apply(msg)
	s.mu.Unlock()
	if changed {
		s.notify()
	}
	return err
}

// apply requires s.mu. It reports whether the mirror changed.
func (d *deviceState) apply(msg protocol.Message) (bool, error) {
	switch msg.Type {
	case protocol.TypePluginRegister:
		var reg protocol.PluginRegister
		if err := msg.PayloadInto(&reg); err != nil {
			return false, fmt.Errorf("plugins: register: %w", err)
		}
		if !pluginID.MatchString(reg.ID) || reg.Name == "" || len(reg.Name) > 64 ||
			len(reg.Description) > 280 || len(reg.Version) > 32 {
			return false, fmt.Errorf("%w: id %q: needs a valid id, a name of 1-64 characters, a description of at most 280 and a version of at most 32", ErrInvalidPlugin, reg.ID)
		}
		p, known := d.plugins[reg.ID]
		if !known && len(d.order) >= MaxPlugins {
			return false, fmt.Errorf("%w: more than %d plugins", ErrInvalidPlugin, MaxPlugins)
		}
		if !known {
			p = &Plugin{ID: reg.ID, Blocks: json.RawMessage(`[]`)}
			d.plugins[reg.ID] = p
			d.order = append(d.order, reg.ID)
		}
		p.Name, p.Description, p.Version = reg.Name, reg.Description, reg.Version
		return true, nil
	case protocol.TypePluginView:
		if len(msg.Payload) > MaxViewBytes {
			var head struct {
				PluginID string `json:"pluginId"`
			}
			_ = json.Unmarshal(msg.Payload, &head) // blocks are decoded below only when small enough
			return d.reject(head.PluginID, "The plugin's view is larger than 1 MiB.")
		}
		var v protocol.PluginView
		if err := msg.PayloadInto(&v); err != nil {
			return false, fmt.Errorf("plugins: view: %w", err)
		}
		if len(v.Blocks) > MaxBlocks {
			return d.reject(v.PluginID, fmt.Sprintf("The plugin's view has more than %d blocks.", MaxBlocks))
		}
		p, ok := d.plugins[v.PluginID]
		if !ok {
			return false, fmt.Errorf("%w: %q", ErrUnknownPlugin, v.PluginID)
		}
		blocks, err := json.Marshal(v.Blocks)
		if err != nil {
			return false, fmt.Errorf("plugins: view: %w", err)
		}
		if v.Blocks == nil {
			blocks = json.RawMessage(`[]`)
		}
		p.Blocks, p.Error, p.UpdatedAt = blocks, "", time.Now()
		return true, nil
	case protocol.TypePluginResult:
		var res protocol.PluginResult
		if err := msg.PayloadInto(&res); err != nil {
			return false, fmt.Errorf("plugins: result: %w", err)
		}
		if ch, ok := d.pending[res.ID]; ok {
			ch <- res
			delete(d.pending, res.ID)
		}
		return false, nil
	default:
		return false, nil
	}
}

// reject replaces a plugin's view with an error.
func (d *deviceState) reject(id, reason string) (bool, error) {
	p, ok := d.plugins[id]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownPlugin, id)
	}
	p.Blocks, p.Error, p.UpdatedAt = json.RawMessage(`[]`), reason, time.Now()
	return true, nil
}

// Devices lists connected devices with the plugins feature, oldest first.
func (s *Service) Devices() []Device {
	s.mu.Lock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d.info)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ConnectedAt.Before(out[j].ConnectedAt) })
	return out
}

// Plugins returns a device's plugins in registration order.
func (s *Service) Plugins(key string) ([]Plugin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[key]
	if !ok {
		return nil, ErrUnknownDevice
	}
	out := make([]Plugin, 0, len(d.order))
	for _, id := range d.order {
		p := *d.plugins[id]
		p.Blocks = slices.Clone(p.Blocks)
		out = append(out, p)
	}
	return out, nil
}

// Action sends the click on actionID of block blockID (and rowID for a
// table row) to the app and returns its message. Ids use the "p" prefix so
// they never look like another feature's.
func (s *Service) Action(ctx context.Context, key, pluginID, blockID, actionID, rowID string) (string, error) {
	if blockID == "" || actionID == "" {
		return "", fmt.Errorf("%w: block and action ids are required", ErrInvalidAction)
	}
	s.mu.Lock()
	d, ok := s.devices[key]
	if !ok {
		s.mu.Unlock()
		return "", ErrUnknownDevice
	}
	if _, ok := d.plugins[pluginID]; !ok {
		s.mu.Unlock()
		return "", fmt.Errorf("%w: %q", ErrUnknownPlugin, pluginID)
	}
	a := protocol.PluginAction{
		ID: fmt.Sprintf("p%d", s.nextID.Add(1)), PluginID: pluginID,
		BlockID: blockID, ActionID: actionID, RowID: rowID,
	}
	ch := make(chan protocol.PluginResult, 1)
	d.pending[a.ID] = ch
	sender := d.sender
	s.mu.Unlock()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.ActionTimeout)
		defer cancel()
	}
	if err := sender.Send(protocol.TypePluginAction, a.ID, a); err != nil {
		s.dropPending(key, a.ID)
		return "", err
	}
	select {
	case res, open := <-ch:
		if !open {
			return "", ErrDisconnected
		}
		if !res.OK {
			return "", fmt.Errorf("%w: %s", ErrActionFailed, res.Error)
		}
		return res.Message, nil
	case <-ctx.Done():
		s.dropPending(key, a.ID)
		return "", fmt.Errorf("plugins: %s: %w", actionID, ctx.Err())
	}
}

func (s *Service) dropPending(key, id string) {
	s.mu.Lock()
	if d, ok := s.devices[key]; ok {
		delete(d.pending, id)
	}
	s.mu.Unlock()
}
