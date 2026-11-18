package wailsui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
)

const queryActionTimeout = 15 * time.Second

// errUnknownActionType rejects an action type outside protocol.ActionTypes
// before it costs a device round trip.
var errUnknownActionType = errors.New("unknown action type")

// QueriesService exposes the TanStack Query mirror to the frontend. State is
// pulled after each app.TopicQueriesChanged notice (ADR-0003).
type QueriesService struct {
	core *app.App
}

// NewQueriesService wraps core for the Wails bindings generator.
func NewQueriesService(core *app.App) *QueriesService {
	return &QueriesService{core: core}
}

// QueriesPage is what the panel pulls for one device.
type QueriesPage struct {
	Queries []queries.Query `json:"queries"`
	Counts  queries.Counts  `json:"counts"`
}

// SDKAddr returns the address the SDK server listens on ("" when disabled).
func (s *QueriesService) SDKAddr() string {
	return s.core.SDKAddr()
}

// Devices lists connected SDK devices.
func (s *QueriesService) Devices() []queries.Device {
	devs := s.core.Queries().Devices()
	if devs == nil {
		devs = []queries.Device{}
	}
	return devs
}

// Queries returns the device's queries in devtools order with counts.
func (s *QueriesService) Queries(deviceKey string) (QueriesPage, error) {
	list, counts, err := s.core.Queries().Queries(deviceKey)
	if list == nil {
		list = []queries.Query{}
	}
	return QueriesPage{Queries: list, Counts: counts}, err
}

// Query returns one query with its full data.
func (s *QueriesService) Query(deviceKey, queryHash string) (queries.Query, error) {
	return s.core.Queries().Query(deviceKey, queryHash)
}

// Mutations returns the device's mutations in devtools order.
func (s *QueriesService) Mutations(deviceKey string) ([]protocol.Mutation, error) {
	list, err := s.core.Queries().Mutations(deviceKey)
	if list == nil {
		list = []protocol.Mutation{}
	}
	return list, err
}

// Action runs a devtools action on the device. actionType must be one of
// protocol.ActionTypes; dataJSON is only read for SET_QUERY_DATA and must be
// valid JSON. Both are checked here, before anything reaches the device.
func (s *QueriesService) Action(deviceKey, actionType, queryHash, dataJSON string) error {
	if !protocol.IsActionType(actionType) {
		return fmt.Errorf("%w: %q", errUnknownActionType, actionType)
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryActionTimeout)
	defer cancel()
	var data json.RawMessage
	if dataJSON != "" {
		if !json.Valid([]byte(dataJSON)) {
			return errInvalidJSON
		}
		data = json.RawMessage(dataJSON)
	}
	return s.core.Queries().Action(ctx, deviceKey, actionType, queryHash, data)
}

// SetOnline switches onlineManager on the device.
func (s *QueriesService) SetOnline(deviceKey string, online bool) error {
	return s.core.Queries().SetOnline(deviceKey, online)
}
