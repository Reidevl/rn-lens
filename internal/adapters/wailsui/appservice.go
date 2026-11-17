package wailsui

import "github.com/Reidevl/rn-lens/internal/app"

// AppService exposes build information to the frontend.
type AppService struct {
	core *app.App
}

// NewAppService wraps core for the Wails bindings generator.
func NewAppService(core *app.App) *AppService {
	return &AppService{core: core}
}

// Info returns the name and version of this Lens build.
func (s *AppService) Info() app.Info {
	return s.core.Info()
}
