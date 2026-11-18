// Package storage mirrors the key-value stores an app exposes through
// @rnlens/sdk (AsyncStorage, MMKV, SecureStore) and forwards reads and
// writes to the device. Values are never mirrored: Lens asks for them on
// demand with storage.action GET.
package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// Sender delivers frames to one connected device.
type Sender interface {
	Send(typ, id string, payload any) error
}

// Device describes one connected SDK that announced the storage feature.
type Device struct {
	// Key is "<app id>|<device id>", stable across reconnects.
	Key         string              `json:"key"`
	SessionID   string              `json:"sessionId"`
	App         protocol.AppInfo    `json:"app"`
	Device      protocol.DeviceInfo `json:"device"`
	SDKVersion  string              `json:"sdkVersion"`
	ConnectedAt time.Time           `json:"connectedAt"`
}

// Errors returned by Service.
var (
	ErrUnknownDevice = errors.New("storage: unknown device")
	ErrStaleSession  = errors.New("storage: frame from a replaced session")
	ErrUnknownStore  = errors.New("storage: unknown store")
	ErrInvalidAction = errors.New("storage: invalid action")
	ErrActionFailed  = errors.New("storage: action failed on the device")
	ErrDisconnected  = errors.New("storage: device disconnected before answering")
	ErrInvalidValue  = errors.New("storage: value does not match its type")
)

type deviceState struct {
	info    Device
	sender  Sender
	stores  map[string]*protocol.StorageStore
	order   []string // store ids in snapshot order
	pending map[string]chan protocol.StorageResult
}

// Service is the storage feature. It is safe for concurrent use.
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

// DeviceKey builds the key used for a device (same form as the queries feature).
func DeviceKey(appID, deviceID string) string { return appID + "|" + deviceID }

// Connect registers a device that announced the storage feature, replacing a
// previous one with the same key; others are ignored.
func (s *Service) Connect(sessionID string, hello protocol.Hello, sender Sender) {
	if !slices.Contains(hello.Features, protocol.FeatureStorage) {
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
		stores:  make(map[string]*protocol.StorageStore),
		pending: make(map[string]chan protocol.StorageResult),
	}
	s.mu.Unlock()
	s.emit()
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
		s.emit()
	}
}

func closePending(d *deviceState) {
	for id, ch := range d.pending {
		close(ch)
		delete(d.pending, id)
	}
}

// Message applies one decoded frame from the device. Frames from a session a
// newer connection replaced are refused with ErrStaleSession.
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
		s.emit()
	}
	return err
}

// apply requires s.mu. It reports whether the mirror changed.
func (d *deviceState) apply(msg protocol.Message) (bool, error) {
	switch msg.Type {
	case protocol.TypeStorageSnapshot:
		var snap protocol.StorageSnapshot
		if err := msg.PayloadInto(&snap); err != nil {
			return false, fmt.Errorf("storage: snapshot: %w", err)
		}
		if snap.StoreID == "" {
			d.stores = make(map[string]*protocol.StorageStore)
			d.order = nil
		}
		replaced := false
		for _, st := range snap.Stores {
			if snap.StoreID != "" && st.ID != snap.StoreID {
				continue
			}
			replaced = true
			st.Keys = sortedKeys(st.Keys)
			if _, known := d.stores[st.ID]; !known {
				d.order = append(d.order, st.ID) // a known store keeps its place
			}
			stored := st
			d.stores[st.ID] = &stored
		}
		if snap.StoreID != "" && !replaced {
			// A one-store snapshot without that store: the app no longer has it.
			delete(d.stores, snap.StoreID)
			d.order = slices.DeleteFunc(d.order, func(id string) bool { return id == snap.StoreID })
		}
		return true, nil
	case protocol.TypeStorageEvent:
		var ev protocol.StorageEvent
		if err := msg.PayloadInto(&ev); err != nil {
			return false, fmt.Errorf("storage: event: %w", err)
		}
		st, ok := d.stores[ev.StoreID]
		if !ok {
			return false, ErrUnknownStore
		}
		return applyEvent(st, ev), nil
	case protocol.TypeStorageResult:
		var res protocol.StorageResult
		if err := msg.PayloadInto(&res); err != nil {
			return false, fmt.Errorf("storage: result: %w", err)
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

func applyEvent(st *protocol.StorageStore, ev protocol.StorageEvent) bool {
	switch ev.Kind {
	case protocol.StorageEventCleared:
		st.Keys = nil
	case protocol.StorageEventRemoved:
		st.Keys = slices.DeleteFunc(st.Keys, func(k protocol.StorageKey) bool { return k.Key == ev.Key })
	case protocol.StorageEventSet:
		i := slices.IndexFunc(st.Keys, func(k protocol.StorageKey) bool { return k.Key == ev.Key })
		if i >= 0 {
			st.Keys[i].Size = ev.Size
		} else {
			st.Keys = sortedKeys(append(st.Keys, protocol.StorageKey{Key: ev.Key, Size: ev.Size}))
		}
	default:
		return false
	}
	return true
}

func sortedKeys(keys []protocol.StorageKey) []protocol.StorageKey {
	out := slices.Clone(keys)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Devices lists connected storage-capable devices, oldest connection first.
func (s *Service) Devices() []Device {
	s.mu.Lock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d.info)
	}
	s.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].ConnectedAt.Before(out[j].ConnectedAt) })
	return out
}

// Stores returns copies of a device's stores, in snapshot order.
func (s *Service) Stores(key string) ([]protocol.StorageStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[key]
	if !ok {
		return nil, ErrUnknownDevice
	}
	out := make([]protocol.StorageStore, 0, len(d.order))
	for _, id := range d.order {
		st := *d.stores[id]
		st.Keys = slices.Clone(st.Keys)
		out = append(out, st)
	}
	return out, nil
}

// Get reads one value from the device.
func (s *Service) Get(ctx context.Context, key, storeID, k string) (protocol.StorageValue, error) {
	res, err := s.action(ctx, key, protocol.StorageAction{Type: protocol.StorageGet, StoreID: storeID, Key: k})
	if err != nil {
		return protocol.StorageValue{}, err
	}
	if res.Value == nil {
		return protocol.StorageValue{}, fmt.Errorf("%w: GET answered without a value", ErrActionFailed)
	}
	return *res.Value, nil
}

// Set writes one value on the device.
func (s *Service) Set(ctx context.Context, key, storeID, k string, v protocol.StorageValue) error {
	_, err := s.action(ctx, key, protocol.StorageAction{Type: protocol.StorageSet, StoreID: storeID, Key: k, Value: &v})
	return err
}

// Remove deletes one key on the device.
func (s *Service) Remove(ctx context.Context, key, storeID, k string) error {
	_, err := s.action(ctx, key, protocol.StorageAction{Type: protocol.StorageRemove, StoreID: storeID, Key: k})
	return err
}

// Clear deletes every key of a store on the device.
func (s *Service) Clear(ctx context.Context, key, storeID string) error {
	_, err := s.action(ctx, key, protocol.StorageAction{Type: protocol.StorageClear, StoreID: storeID})
	return err
}

// Refresh asks the device to send a fresh snapshot of a store.
func (s *Service) Refresh(ctx context.Context, key, storeID string) error {
	_, err := s.action(ctx, key, protocol.StorageAction{Type: protocol.StorageRefresh, StoreID: storeID})
	return err
}

// NormalizeValue checks that a value's text matches its type and returns it
// in canonical form: numbers as JSON numbers (no NaN, Inf or hex), booleans
// as true or false, bytes as trimmed base64; strings are kept as they are.
func NormalizeValue(v protocol.StorageValue) (protocol.StorageValue, error) {
	switch v.Type {
	case protocol.StorageValueString:
		return v, nil
	case protocol.StorageValueNumber:
		text := strings.TrimSpace(v.Value)
		var f float64
		if !json.Valid([]byte(text)) || json.Unmarshal([]byte(text), &f) != nil {
			return v, fmt.Errorf("%w: %q is not a number", ErrInvalidValue, v.Value)
		}
		v.Value = strconv.FormatFloat(f, 'g', -1, 64)
	case protocol.StorageValueBoolean:
		switch strings.ToLower(strings.TrimSpace(v.Value)) {
		case "true":
			v.Value = "true"
		case "false":
			v.Value = "false"
		default:
			return v, fmt.Errorf("%w: %q is not true or false", ErrInvalidValue, v.Value)
		}
	case protocol.StorageValueBytes:
		text := strings.TrimSpace(v.Value)
		if _, err := base64.StdEncoding.DecodeString(text); err != nil {
			return v, fmt.Errorf("%w: bytes must be base64", ErrInvalidValue)
		}
		v.Value = text
	default:
		return v, fmt.Errorf("%w: unknown value type %q", ErrInvalidAction, v.Type)
	}
	return v, nil
}

func validate(a protocol.StorageAction) error {
	needsKey := a.Type == protocol.StorageGet || a.Type == protocol.StorageSet || a.Type == protocol.StorageRemove
	switch {
	case !protocol.IsStorageActionType(a.Type):
		return fmt.Errorf("%w: unknown type %q", ErrInvalidAction, a.Type)
	case needsKey && a.Key == "":
		return fmt.Errorf("%w: %s needs a key", ErrInvalidAction, a.Type)
	case a.Type == protocol.StorageSet && a.Value == nil:
		return fmt.Errorf("%w: SET needs a value", ErrInvalidAction)
	}
	return nil
}

// action sends a storage.action and waits for its storage.result. Ids use the
// "s" prefix so they never look like the queries feature's.
func (s *Service) action(ctx context.Context, key string, a protocol.StorageAction) (protocol.StorageResult, error) {
	if err := validate(a); err != nil {
		return protocol.StorageResult{}, err
	}
	if a.Value != nil {
		v, err := NormalizeValue(*a.Value)
		if err != nil {
			return protocol.StorageResult{}, err
		}
		a.Value = &v
	}
	s.mu.Lock()
	d, ok := s.devices[key]
	if !ok {
		s.mu.Unlock()
		return protocol.StorageResult{}, ErrUnknownDevice
	}
	if _, ok := d.stores[a.StoreID]; !ok {
		s.mu.Unlock()
		return protocol.StorageResult{}, fmt.Errorf("%w: %s", ErrUnknownStore, a.StoreID)
	}
	a.ID = fmt.Sprintf("s%d", s.nextID.Add(1))
	ch := make(chan protocol.StorageResult, 1)
	d.pending[a.ID] = ch
	sender := d.sender
	s.mu.Unlock()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.ActionTimeout)
		defer cancel()
	}
	if err := sender.Send(protocol.TypeStorageAction, a.ID, a); err != nil {
		s.dropPending(key, a.ID)
		return protocol.StorageResult{}, err
	}
	select {
	case res, open := <-ch:
		if !open {
			return protocol.StorageResult{}, ErrDisconnected
		}
		if !res.OK {
			return res, fmt.Errorf("%w: %s", ErrActionFailed, res.Error)
		}
		return res, nil
	case <-ctx.Done():
		s.dropPending(key, a.ID)
		return protocol.StorageResult{}, fmt.Errorf("storage: %s: %w", a.Type, ctx.Err())
	}
}

func (s *Service) dropPending(key, id string) {
	s.mu.Lock()
	if d, ok := s.devices[key]; ok {
		delete(d.pending, id)
	}
	s.mu.Unlock()
}

func (s *Service) emit() {
	if s.notify != nil {
		s.notify()
	}
}
