// Package protocol holds the Go side of the Lens SDK protocol (version 1).
//
// The JSON Schema under protocol/schema is the source of truth (ADR-0004);
// these hand-written types are proven against every fixture by a strict
// decode in protocol_test.go, so a field added to the schema without a Go
// counterpart fails the build's tests.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Version is the protocol major this package implements.
const Version = 1

// Type names every message carries in the envelope.
const (
	TypeHello           = "hello"
	TypeWelcome         = "welcome"
	TypeSnapshotRequest = "snapshot.request"
	TypeQueriesSnapshot = "queries.snapshot"
	TypeQueryEvent      = "query.event"
	TypeMutationEvent   = "mutation.event"
	TypeOnlineStatus    = "online.status"
	TypeQueryAction     = "query.action"
	TypeActionResult    = "action.result"
	TypeOnlineSet       = "online.set"
	TypePing            = "ping"
	TypePong            = "pong"
)

// Message is the envelope of every frame.
type Message struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

// ErrVersion is returned when the envelope carries another protocol major.
var ErrVersion = errors.New("protocol: unsupported version")

// Decode parses a frame and checks the version. The payload stays raw until
// the caller picks the typed accessor for msg.Type.
func Decode(raw []byte) (Message, error) {
	var m Message
	if err := strictUnmarshal(raw, &m); err != nil {
		return Message{}, fmt.Errorf("protocol: decode envelope: %w", err)
	}
	if m.V != Version {
		return m, fmt.Errorf("%w: got %d, want %d", ErrVersion, m.V, Version)
	}
	if m.Type == "" {
		return m, errors.New("protocol: missing type")
	}
	return m, nil
}

// Encode builds a frame around payload.
func Encode(typ, id string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("protocol: encode %s payload: %w", typ, err)
	}
	return json.Marshal(Message{V: Version, Type: typ, ID: id, Payload: raw})
}

// PayloadInto strictly decodes the payload into dst, rejecting unknown fields.
func (m Message) PayloadInto(dst any) error {
	if err := strictUnmarshal(m.Payload, dst); err != nil {
		return fmt.Errorf("protocol: decode %s payload: %w", m.Type, err)
	}
	return nil
}

func strictUnmarshal(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	return nil
}

// Platform values AppInfo.Platform may take.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
	PlatformWeb     = "web"
	PlatformMacOS   = "macos"
	PlatformWindows = "windows"
	PlatformOther   = "other"
)

// AppInfo identifies the application the SDK runs in.
type AppInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Platform  string `json:"platform"`
	RNVersion string `json:"rnVersion,omitempty"`
}

// DeviceInfo identifies the device or simulator.
type DeviceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Features the SDK can announce in hello.
const (
	FeatureQueries = "queries"
	FeatureStorage = "storage"
	FeaturePlugins = "plugins"
)

// Hello is the first frame the SDK sends.
type Hello struct {
	SDKVersion      string     `json:"sdkVersion"`
	ProtocolVersion int        `json:"protocolVersion"`
	App             AppInfo    `json:"app"`
	Device          DeviceInfo `json:"device"`
	ScriptURL       string     `json:"scriptURL,omitempty"`
	Features        []string   `json:"features"`
	Token           string     `json:"token,omitempty"`
}

// Welcome acknowledges a hello.
type Welcome struct {
	LensVersion     string `json:"lensVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
	SessionID       string `json:"sessionId"`
}

// Empty is the payload of ping, pong and snapshot.request.
type Empty struct{}

// QueryState mirrors TanStack's query state. JSON-typed fields stay raw so
// large data is never re-encoded on the way to the UI.
type QueryState struct {
	Status            string          `json:"status"`
	FetchStatus       string          `json:"fetchStatus"`
	DataUpdatedAt     float64         `json:"dataUpdatedAt"`
	ErrorUpdatedAt    float64         `json:"errorUpdatedAt"`
	DataUpdateCount   int             `json:"dataUpdateCount"`
	ErrorUpdateCount  int             `json:"errorUpdateCount"`
	FetchFailureCount int             `json:"fetchFailureCount"`
	IsInvalidated     bool            `json:"isInvalidated"`
	Data              json.RawMessage `json:"data,omitempty"`
	Error             json.RawMessage `json:"error,omitempty"`
	FetchMeta         json.RawMessage `json:"fetchMeta,omitempty"`
}

// QueryStateDelta is the subset of QueryState an update carries. Pointers
// distinguish "unchanged" from a zero value.
type QueryStateDelta struct {
	Status            *string         `json:"status,omitempty"`
	FetchStatus       *string         `json:"fetchStatus,omitempty"`
	DataUpdatedAt     *float64        `json:"dataUpdatedAt,omitempty"`
	ErrorUpdatedAt    *float64        `json:"errorUpdatedAt,omitempty"`
	DataUpdateCount   *int            `json:"dataUpdateCount,omitempty"`
	ErrorUpdateCount  *int            `json:"errorUpdateCount,omitempty"`
	FetchFailureCount *int            `json:"fetchFailureCount,omitempty"`
	IsInvalidated     *bool           `json:"isInvalidated,omitempty"`
	Data              json.RawMessage `json:"data,omitempty"`
	Error             json.RawMessage `json:"error,omitempty"`
	FetchMeta         json.RawMessage `json:"fetchMeta,omitempty"`
}

// Query is one entry of the query cache.
type Query struct {
	QueryHash      string          `json:"queryHash"`
	QueryKey       json.RawMessage `json:"queryKey"`
	State          QueryState      `json:"state"`
	ObserversCount int             `json:"observersCount"`
	IsActive       bool            `json:"isActive"`
	IsStale        bool            `json:"isStale"`
	GCTime         *float64        `json:"gcTime,omitempty"`
	Meta           json.RawMessage `json:"meta,omitempty"`
	DataTruncated  bool            `json:"dataTruncated,omitempty"`
}

// MutationState mirrors TanStack's mutation state.
type MutationState struct {
	Status        string          `json:"status"`
	SubmittedAt   float64         `json:"submittedAt"`
	IsPaused      bool            `json:"isPaused"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	Error         json.RawMessage `json:"error,omitempty"`
	FailureCount  int             `json:"failureCount,omitempty"`
	FailureReason json.RawMessage `json:"failureReason,omitempty"`
	Context       json.RawMessage `json:"context,omitempty"`
}

// MutationStateDelta is the subset of MutationState an update carries.
type MutationStateDelta struct {
	Status        *string         `json:"status,omitempty"`
	SubmittedAt   *float64        `json:"submittedAt,omitempty"`
	IsPaused      *bool           `json:"isPaused,omitempty"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	Error         json.RawMessage `json:"error,omitempty"`
	FailureCount  *int            `json:"failureCount,omitempty"`
	FailureReason json.RawMessage `json:"failureReason,omitempty"`
	Context       json.RawMessage `json:"context,omitempty"`
}

// Mutation is one entry of the mutation cache.
type Mutation struct {
	MutationID    int             `json:"mutationId"`
	MutationKey   json.RawMessage `json:"mutationKey,omitempty"`
	State         MutationState   `json:"state"`
	Scope         json.RawMessage `json:"scope,omitempty"`
	Meta          json.RawMessage `json:"meta,omitempty"`
	DataTruncated bool            `json:"dataTruncated,omitempty"`
}

// QueriesSnapshot carries the whole cache.
type QueriesSnapshot struct {
	Queries   []Query    `json:"queries"`
	Mutations []Mutation `json:"mutations"`
	Online    bool       `json:"online"`
}

// Query event kinds.
const (
	EventAdded     = "added"
	EventUpdated   = "updated"
	EventRemoved   = "removed"
	EventObservers = "observers"
)

// QueryEvent is one query cache change.
type QueryEvent struct {
	Kind           string           `json:"kind"`
	QueryHash      string           `json:"queryHash"`
	Query          *Query           `json:"query,omitempty"`
	Action         string           `json:"action,omitempty"`
	Delta          *QueryStateDelta `json:"delta,omitempty"`
	ObserversCount *int             `json:"observersCount,omitempty"`
	IsActive       *bool            `json:"isActive,omitempty"`
	IsStale        *bool            `json:"isStale,omitempty"`
}

// MutationEvent is one mutation cache change.
type MutationEvent struct {
	Kind       string              `json:"kind"`
	MutationID int                 `json:"mutationId"`
	Mutation   *Mutation           `json:"mutation,omitempty"`
	Delta      *MutationStateDelta `json:"delta,omitempty"`
}

// OnlineStatus reports onlineManager on the device.
type OnlineStatus struct {
	Online bool `json:"online"`
}

// OnlineSet asks the device to switch onlineManager.
type OnlineSet struct {
	Online bool `json:"online"`
}

// Action types Lens can ask the device to run.
const (
	ActionRefetch            = "REFETCH"
	ActionInvalidate         = "INVALIDATE"
	ActionReset              = "RESET"
	ActionRemove             = "REMOVE"
	ActionSetQueryData       = "SET_QUERY_DATA"
	ActionTriggerError       = "TRIGGER_ERROR"
	ActionRestoreError       = "RESTORE_ERROR"
	ActionTriggerLoading     = "TRIGGER_LOADING"
	ActionRestoreLoading     = "RESTORE_LOADING"
	ActionClearQueryCache    = "CLEAR_QUERY_CACHE"
	ActionClearMutationCache = "CLEAR_MUTATION_CACHE"
)

// ActionTypes lists every valid action, in schema order.
var ActionTypes = []string{
	ActionRefetch, ActionInvalidate, ActionReset, ActionRemove, ActionSetQueryData,
	ActionTriggerError, ActionRestoreError, ActionTriggerLoading, ActionRestoreLoading,
	ActionClearQueryCache, ActionClearMutationCache,
}

// QueryAction asks the device to run a devtools action.
type QueryAction struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	QueryHash string          `json:"queryHash,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// ActionResult answers a QueryAction.
type ActionResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// PayloadType returns a pointer to the Go type that decodes typ's payload,
// or nil for unknown types.
func PayloadType(typ string) any {
	switch typ {
	case TypeHello:
		return &Hello{}
	case TypeWelcome:
		return &Welcome{}
	case TypeSnapshotRequest, TypePing, TypePong:
		return &Empty{}
	case TypeQueriesSnapshot:
		return &QueriesSnapshot{}
	case TypeQueryEvent:
		return &QueryEvent{}
	case TypeMutationEvent:
		return &MutationEvent{}
	case TypeOnlineStatus:
		return &OnlineStatus{}
	case TypeQueryAction:
		return &QueryAction{}
	case TypeActionResult:
		return &ActionResult{}
	case TypeOnlineSet:
		return &OnlineSet{}
	default:
		return nil
	}
}
