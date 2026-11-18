package wailsui

import (
	"context"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/plugins"
)

// pluginActionTimeout bounds one click on a plugin action.
const pluginActionTimeout = 15 * time.Second

// PluginsService exposes the apps' plugin panels to the frontend. The UI
// pulls Devices then Plugins after each app.TopicPluginsChanged notice.
type PluginsService struct {
	core *app.App
}

// NewPluginsService wraps core.
func NewPluginsService(core *app.App) *PluginsService {
	return &PluginsService{core: core}
}

// Devices lists connected devices that have plugins.
func (s *PluginsService) Devices() []plugins.Device {
	devs := s.core.Plugins().Devices()
	if devs == nil {
		devs = []plugins.Device{}
	}
	return devs
}

// Plugins returns a device's plugins and their panels.
func (s *PluginsService) Plugins(deviceKey string) ([]plugins.Plugin, error) {
	list, err := s.core.Plugins().Plugins(deviceKey)
	if list == nil {
		list = []plugins.Plugin{}
	}
	return list, err
}

// Action sends a click on a plugin action to the app and returns the
// message the app answered with ("" when none).
func (s *PluginsService) Action(deviceKey, pluginID, blockID, actionID, rowID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pluginActionTimeout)
	defer cancel()
	return s.core.Plugins().Action(ctx, deviceKey, pluginID, blockID, actionID, rowID)
}
