package wailsui

import (
	"errors"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
)

// Action validates its input before asking the device: an unknown action
// type or malformed data never costs a round trip. With no device connected,
// a request that passes validation surfaces the service's own error.
func TestActionValidatesInputBeforeReachingTheDevice(t *testing.T) {
	t.Parallel()
	svc := NewQueriesService(app.New(app.Config{SDKAddr: "off"}))
	tests := []struct {
		name       string
		actionType string
		data       string
		want       error
	}{
		{name: "unknown action type", actionType: "EXPLODE", data: "", want: errUnknownActionType},
		{name: "empty action type", actionType: "", data: "", want: errUnknownActionType},
		{name: "data that is not JSON", actionType: protocol.ActionSetQueryData, data: "{not json", want: errInvalidJSON},
		{name: "valid action reaches the service", actionType: protocol.ActionRefetch, data: "", want: queries.ErrUnknownDevice},
		{name: "valid action with data reaches the service", actionType: protocol.ActionSetQueryData, data: `{"a":1}`, want: queries.ErrUnknownDevice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := svc.Action("no-such-device", tt.actionType, "hash", tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Action(%q, %q) = %v, want %v", tt.actionType, tt.data, err, tt.want)
			}
		})
	}
}
