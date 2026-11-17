package wailsui

import (
	"context"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/targets"
)

const actionTimeout = 5 * time.Second

// TargetsService exposes target discovery and the attach session to the
// frontend. State is pulled; change notifications arrive as events named
// app.TopicTargetsChanged and app.TopicSessionChanged.
type TargetsService struct {
	core *app.App
}

// NewTargetsService wraps core for the Wails bindings generator.
func NewTargetsService(core *app.App) *TargetsService {
	return &TargetsService{core: core}
}

// List returns every discovered target.
func (s *TargetsService) List() []targets.Target {
	list := s.core.Targets()
	if list == nil {
		list = []targets.Target{}
	}
	return list
}

// Refresh polls Metro immediately instead of waiting for the next tick.
func (s *TargetsService) Refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	s.core.RefreshTargets(ctx)
}

// Session returns the current session snapshot.
func (s *TargetsService) Session() targets.Snapshot {
	return s.core.SessionSnapshot()
}

// Attach starts a session with the target.
func (s *TargetsService) Attach(id string) error {
	return s.core.Attach(id)
}

// Detach ends the current session.
func (s *TargetsService) Detach() {
	s.core.Detach()
}

// Reload reloads the attached app's JavaScript.
func (s *TargetsService) Reload() error {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.Reload(ctx)
}

// OpenInReactNativeDevTools asks Metro to open React Native DevTools for the target.
func (s *TargetsService) OpenInReactNativeDevTools(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.OpenInReactNativeDevTools(ctx, id)
}
