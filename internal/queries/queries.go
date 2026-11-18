// Package queries mirrors the TanStack Query caches of every app that
// connected through @rnlens/sdk and forwards devtools actions back to them.
// It knows nothing about WebSockets: the composition root feeds it decoded
// protocol frames and a Sender per device.
package queries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// MaxDataBytes is the size above which a query's data is replaced by a marker.
const MaxDataBytes = 1 << 20

// Sender delivers frames to one connected device.
type Sender interface {
	Send(typ, id string, payload any) error
}

// Device describes one connected SDK.
type Device struct {
	// Key is "<app id>|<device id>", stable across reconnects.
	Key         string              `json:"key"`
	SessionID   string              `json:"sessionId"`
	App         protocol.AppInfo    `json:"app"`
	Device      protocol.DeviceInfo `json:"device"`
	SDKVersion  string              `json:"sdkVersion"`
	Features    []string            `json:"features"`
	ConnectedAt time.Time           `json:"connectedAt"`
	// LastMessageAt is when the device last sent a frame (any feature);
	// pings are not counted. The connection itself is kept alive by pings,
	// so a listed device is connected even when it is idle.
	LastMessageAt time.Time `json:"lastMessageAt"`
	Online        bool      `json:"online"`
}

// Label is the devtools status bucket of a query.
type Label string

// Labels, in the order the official devtools rank them.
const (
	LabelFetching Label = "fetching"
	LabelFresh    Label = "fresh"
	LabelStale    Label = "stale"
	LabelInactive Label = "inactive"
	LabelPaused   Label = "paused"
)

// Query is a mirrored query with its derived label.
type Query struct {
	protocol.Query
	Label Label `json:"label"`
}

// LabelOf applies the official devtools rule.
func LabelOf(q protocol.Query) Label {
	switch {
	case q.State.FetchStatus == "fetching":
		return LabelFetching
	case q.ObserversCount == 0:
		return LabelInactive
	case q.State.FetchStatus == "paused":
		return LabelPaused
	case q.IsStale:
		return LabelStale
	default:
		return LabelFresh
	}
}

func labelRank(l Label) int {
	switch l {
	case LabelFetching, LabelPaused:
		return 0
	case LabelFresh:
		return 1
	case LabelStale:
		return 2
	case LabelInactive:
		return 3
	default:
		return 3
	}
}

func mutationRank(m protocol.Mutation) int {
	switch {
	case m.State.IsPaused:
		return 0
	case m.State.Status == "pending":
		return 1
	case m.State.Status == "error":
		return 2
	default:
		return 3
	}
}

// Counts summarises queries per label.
type Counts struct {
	Fetching int `json:"fetching"`
	Fresh    int `json:"fresh"`
	Stale    int `json:"stale"`
	Inactive int `json:"inactive"`
	Paused   int `json:"paused"`
	Total    int `json:"total"`
}

// Errors returned by Service.
var (
	ErrUnknownDevice = errors.New("queries: unknown device")
	ErrStaleSession  = errors.New("queries: frame from a replaced session")
	ErrActionFailed  = errors.New("queries: action failed on the device")
)

type deviceState struct {
	info      Device
	sender    Sender
	queries   map[string]protocol.Query
	mutations map[int]protocol.Mutation
	pending   map[string]chan protocol.ActionResult
}

// Service is the queries feature. It is safe for concurrent use.
type Service struct {
	mu      sync.RWMutex
	devices map[string]*deviceState
	notify  func()
	nextID  atomic.Uint64
	// ActionTimeout bounds Action when the context has no deadline.
	ActionTimeout time.Duration
}

// NewService creates an empty service; notify runs after every change.
func NewService(notify func()) *Service {
	return &Service{devices: make(map[string]*deviceState), notify: notify, ActionTimeout: 10 * time.Second}
}

// DeviceKey builds the key used everywhere for a device.
func DeviceKey(appID, deviceID string) string { return appID + "|" + deviceID }

// Seen records that the device's session sent a frame now. Frames of a
// replaced session and unknown devices are ignored. It does not notify:
// activity alone changes nothing the panels show.
func (s *Service) Seen(key, sessionID string) {
	s.mu.Lock()
	if d, ok := s.devices[key]; ok && d.info.SessionID == sessionID {
		d.info.LastMessageAt = time.Now()
	}
	s.mu.Unlock()
}

// Connect registers a device (replacing a previous one with the same key).
func (s *Service) Connect(sessionID string, hello protocol.Hello, sender Sender) Device {
	key := DeviceKey(hello.App.ID, hello.Device.ID)
	now := time.Now()
	info := Device{
		Key: key, SessionID: sessionID, App: hello.App, Device: hello.Device,
		SDKVersion: hello.SDKVersion, Features: hello.Features, ConnectedAt: now, LastMessageAt: now, Online: true,
	}
	s.mu.Lock()
	if old, ok := s.devices[key]; ok {
		for id, ch := range old.pending {
			close(ch)
			delete(old.pending, id)
		}
	}
	s.devices[key] = &deviceState{
		info: info, sender: sender,
		queries:   make(map[string]protocol.Query),
		mutations: make(map[int]protocol.Mutation),
		pending:   make(map[string]chan protocol.ActionResult),
	}
	s.mu.Unlock()
	s.emit()
	return info
}

// Disconnect forgets a device if sessionID still owns it.
func (s *Service) Disconnect(key, sessionID string) {
	s.mu.Lock()
	d, ok := s.devices[key]
	removed := ok && d.info.SessionID == sessionID
	if removed {
		for id, ch := range d.pending {
			close(ch)
			delete(d.pending, id)
		}
		delete(s.devices, key)
	}
	s.mu.Unlock()
	if removed {
		s.emit()
	}
}

// Message applies one decoded frame from the device. sessionID names the
// connection that carried the frame: a frame from a session that a newer
// connection replaced is ignored with ErrStaleSession so it can never land on
// the mirror the new session owns.
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

func (d *deviceState) apply(msg protocol.Message) (bool, error) {
	switch msg.Type {
	case protocol.TypeQueriesSnapshot:
		var p protocol.QueriesSnapshot
		if err := msg.PayloadInto(&p); err != nil {
			return false, err
		}
		d.queries = make(map[string]protocol.Query, len(p.Queries))
		for _, q := range p.Queries {
			d.queries[q.QueryHash] = capData(q)
		}
		d.mutations = make(map[int]protocol.Mutation, len(p.Mutations))
		for _, m := range p.Mutations {
			d.mutations[m.MutationID] = capMutation(m)
		}
		d.info.Online = p.Online
		return true, nil

	case protocol.TypeQueryEvent:
		var ev protocol.QueryEvent
		if err := msg.PayloadInto(&ev); err != nil {
			return false, err
		}
		return d.applyQueryEvent(ev), nil

	case protocol.TypeMutationEvent:
		var ev protocol.MutationEvent
		if err := msg.PayloadInto(&ev); err != nil {
			return false, err
		}
		return d.applyMutationEvent(ev), nil

	case protocol.TypeOnlineStatus:
		var p protocol.OnlineStatus
		if err := msg.PayloadInto(&p); err != nil {
			return false, err
		}
		d.info.Online = p.Online
		return true, nil

	case protocol.TypeActionResult:
		var p protocol.ActionResult
		if err := msg.PayloadInto(&p); err != nil {
			return false, err
		}
		if ch, ok := d.pending[p.ID]; ok {
			ch <- p
			delete(d.pending, p.ID)
		}
		return false, nil

	default:
		return false, nil
	}
}

func (d *deviceState) applyQueryEvent(ev protocol.QueryEvent) bool {
	switch ev.Kind {
	case protocol.EventAdded:
		if ev.Query == nil {
			return false
		}
		d.queries[ev.QueryHash] = capData(*ev.Query)
		return true
	case protocol.EventRemoved:
		_, ok := d.queries[ev.QueryHash]
		delete(d.queries, ev.QueryHash)
		return ok
	case protocol.EventUpdated:
		if ev.Query != nil {
			d.queries[ev.QueryHash] = capData(*ev.Query)
			return true
		}
		q, ok := d.queries[ev.QueryHash]
		if !ok || ev.Delta == nil {
			return false
		}
		applyDelta(&q.State, ev.Delta)
		d.queries[ev.QueryHash] = capData(q)
		return true
	case protocol.EventObservers:
		q, ok := d.queries[ev.QueryHash]
		if !ok {
			return false
		}
		if ev.ObserversCount != nil {
			q.ObserversCount = *ev.ObserversCount
		}
		if ev.IsActive != nil {
			q.IsActive = *ev.IsActive
		}
		if ev.IsStale != nil {
			q.IsStale = *ev.IsStale
		}
		d.queries[ev.QueryHash] = q
		return true
	default:
		return false
	}
}

func applyDelta(st *protocol.QueryState, dl *protocol.QueryStateDelta) {
	if dl.Status != nil {
		st.Status = *dl.Status
	}
	if dl.FetchStatus != nil {
		st.FetchStatus = *dl.FetchStatus
	}
	if dl.DataUpdatedAt != nil {
		st.DataUpdatedAt = *dl.DataUpdatedAt
	}
	if dl.ErrorUpdatedAt != nil {
		st.ErrorUpdatedAt = *dl.ErrorUpdatedAt
	}
	if dl.DataUpdateCount != nil {
		st.DataUpdateCount = *dl.DataUpdateCount
	}
	if dl.ErrorUpdateCount != nil {
		st.ErrorUpdateCount = *dl.ErrorUpdateCount
	}
	if dl.FetchFailureCount != nil {
		st.FetchFailureCount = *dl.FetchFailureCount
	}
	if dl.IsInvalidated != nil {
		st.IsInvalidated = *dl.IsInvalidated
	}
	applyRaw(&st.Data, dl.Data)
	applyRaw(&st.Error, dl.Error)
	applyRaw(&st.FetchMeta, dl.FetchMeta)
}

// applyRaw applies one JSON-typed delta field: absent (nil) leaves dst
// untouched, JSON null clears it (the device set the field to undefined, as
// TRIGGER_LOADING, TRIGGER_ERROR and RESET do) and any other value replaces it.
func applyRaw(dst *json.RawMessage, src json.RawMessage) {
	switch {
	case src == nil:
	case isJSONNull(src):
		*dst = nil
	default:
		*dst = src
	}
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func (d *deviceState) applyMutationEvent(ev protocol.MutationEvent) bool {
	switch ev.Kind {
	case protocol.EventAdded:
		if ev.Mutation == nil {
			return false
		}
		d.mutations[ev.MutationID] = capMutation(*ev.Mutation)
		return true
	case protocol.EventRemoved:
		_, ok := d.mutations[ev.MutationID]
		delete(d.mutations, ev.MutationID)
		return ok
	case protocol.EventUpdated:
		if ev.Mutation != nil {
			d.mutations[ev.MutationID] = capMutation(*ev.Mutation)
			return true
		}
		m, ok := d.mutations[ev.MutationID]
		if !ok || ev.Delta == nil {
			return false
		}
		dl := ev.Delta
		if dl.Status != nil {
			m.State.Status = *dl.Status
		}
		if dl.SubmittedAt != nil {
			m.State.SubmittedAt = *dl.SubmittedAt
		}
		if dl.IsPaused != nil {
			m.State.IsPaused = *dl.IsPaused
		}
		applyRaw(&m.State.Variables, dl.Variables)
		applyRaw(&m.State.Data, dl.Data)
		applyRaw(&m.State.Error, dl.Error)
		if dl.FailureCount != nil {
			m.State.FailureCount = *dl.FailureCount
		}
		applyRaw(&m.State.FailureReason, dl.FailureReason)
		applyRaw(&m.State.Context, dl.Context)
		d.mutations[ev.MutationID] = capMutation(m)
		return true
	default:
		return false
	}
}

// capData enforces MaxDataBytes on the server side too and derives
// DataTruncated from the data on every apply: true when Lens capped it here or
// when the SDK already replaced it with its truncation marker, false otherwise,
// so a later delta that carries real data clears the flag.
func capData(q protocol.Query) protocol.Query {
	if len(q.State.Data) > MaxDataBytes {
		q.State.Data = json.RawMessage(fmt.Sprintf(`{"__lensTruncated":true,"bytes":%d}`, len(q.State.Data)))
	}
	q.DataTruncated = isTruncationMarker(q.State.Data)
	return q
}

// capMutation enforces MaxDataBytes on the mutation payloads that can carry
// arbitrary user data; the marker shape matches the SDK's.
func capMutation(m protocol.Mutation) protocol.Mutation {
	m.State.Variables = capRaw(m.State.Variables)
	m.State.Data = capRaw(m.State.Data)
	m.State.Context = capRaw(m.State.Context)
	return m
}

func capRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) > MaxDataBytes {
		return json.RawMessage(fmt.Sprintf(`{"__lensTruncated":true,"bytes":%d}`, len(raw)))
	}
	return raw
}

// isTruncationMarker reports whether data is the `{"__lensTruncated":true,...}`
// placeholder the SDK and Lens substitute for oversized values.
func isTruncationMarker(data json.RawMessage) bool {
	var marker struct {
		Truncated bool `json:"__lensTruncated"`
	}
	return len(data) > 0 && data[0] == '{' && json.Unmarshal(data, &marker) == nil && marker.Truncated
}

// Devices lists connected devices, oldest connection first.
func (s *Service) Devices() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConnectedAt.Before(out[j].ConnectedAt) })
	return out
}

// Queries returns the device's queries in devtools order with counts.
func (s *Service) Queries(key string) ([]Query, Counts, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[key]
	if !ok {
		return nil, Counts{}, ErrUnknownDevice
	}
	out := make([]Query, 0, len(d.queries))
	var c Counts
	for _, q := range d.queries {
		l := LabelOf(q)
		out = append(out, Query{Query: q, Label: l})
		c.Total++
		switch l {
		case LabelFetching:
			c.Fetching++
		case LabelFresh:
			c.Fresh++
		case LabelStale:
			c.Stale++
		case LabelInactive:
			c.Inactive++
		case LabelPaused:
			c.Paused++
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := labelRank(out[i].Label), labelRank(out[j].Label)
		if ri != rj {
			return ri < rj
		}
		if out[i].State.DataUpdatedAt != out[j].State.DataUpdatedAt {
			return out[i].State.DataUpdatedAt > out[j].State.DataUpdatedAt
		}
		return out[i].QueryHash < out[j].QueryHash
	})
	return out, c, nil
}

// Query returns one query by hash.
func (s *Service) Query(key, hash string) (Query, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[key]
	if !ok {
		return Query{}, ErrUnknownDevice
	}
	q, ok := d.queries[hash]
	if !ok {
		return Query{}, fmt.Errorf("queries: no query %q", hash)
	}
	return Query{Query: q, Label: LabelOf(q)}, nil
}

// Mutations returns the device's mutations in devtools order.
func (s *Service) Mutations(key string) ([]protocol.Mutation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[key]
	if !ok {
		return nil, ErrUnknownDevice
	}
	out := make([]protocol.Mutation, 0, len(d.mutations))
	for _, m := range d.mutations {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := mutationRank(out[i]), mutationRank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].State.SubmittedAt > out[j].State.SubmittedAt
	})
	return out, nil
}

// Action asks the device to run a devtools action and waits for its result.
func (s *Service) Action(ctx context.Context, key, actionType, hash string, data json.RawMessage) error {
	s.mu.Lock()
	d, ok := s.devices[key]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownDevice
	}
	id := fmt.Sprintf("a%d", s.nextID.Add(1))
	ch := make(chan protocol.ActionResult, 1)
	d.pending[id] = ch
	sender := d.sender
	s.mu.Unlock()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.ActionTimeout)
		defer cancel()
	}
	err := sender.Send(protocol.TypeQueryAction, id, protocol.QueryAction{ID: id, Type: actionType, QueryHash: hash, Data: data})
	if err != nil {
		s.dropPending(key, id)
		return err
	}
	select {
	case res, open := <-ch:
		if !open {
			return errors.New("queries: device disconnected before answering")
		}
		if !res.OK {
			return fmt.Errorf("%w: %s", ErrActionFailed, res.Error)
		}
		return nil
	case <-ctx.Done():
		s.dropPending(key, id)
		return fmt.Errorf("queries: action %s: %w", actionType, ctx.Err())
	}
}

func (s *Service) dropPending(key, id string) {
	s.mu.Lock()
	if d, ok := s.devices[key]; ok {
		delete(d.pending, id)
	}
	s.mu.Unlock()
}

// SetOnline asks the device to switch onlineManager and mirrors the flag.
func (s *Service) SetOnline(key string, online bool) error {
	s.mu.Lock()
	d, ok := s.devices[key]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownDevice
	}
	sender := d.sender
	s.mu.Unlock()
	if err := sender.Send(protocol.TypeOnlineSet, "", protocol.OnlineSet{Online: online}); err != nil {
		return err
	}
	s.mu.Lock()
	if d, ok := s.devices[key]; ok {
		d.info.Online = online
	}
	s.mu.Unlock()
	s.emit()
	return nil
}

func (s *Service) emit() {
	if s.notify != nil {
		s.notify()
	}
}
