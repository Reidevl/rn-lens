package wailsui

import (
	"errors"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/plugins"
)

func TestPluginsServiceReturnsEmptyListsAndErrors(t *testing.T) {
	t.Parallel()
	svc := NewPluginsService(app.New(app.Config{SDKAddr: "off"}))
	if devs := svc.Devices(); devs == nil || len(devs) != 0 {
		t.Fatalf("Devices = %#v, want an empty slice for the frontend", devs)
	}
	list, err := svc.Plugins("nope|d")
	if list == nil || !errors.Is(err, plugins.ErrUnknownDevice) {
		t.Fatalf("Plugins = %#v, %v", list, err)
	}
	if _, err := svc.Action("nope|d", "p", "b", "a", ""); !errors.Is(err, plugins.ErrUnknownDevice) {
		t.Fatalf("Action err = %v", err)
	}
}
