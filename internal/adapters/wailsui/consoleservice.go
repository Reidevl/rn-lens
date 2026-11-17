package wailsui

import (
	"context"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/console"
)

const evalTimeout = 30 * time.Second

// ConsoleService exposes the console log to the frontend. The UI pulls
// Stats then Range after each app.TopicConsoleChanged notice (ADR-0003).
type ConsoleService struct {
	core *app.App
}

// NewConsoleService wraps core for the Wails bindings generator.
func NewConsoleService(core *app.App) *ConsoleService {
	return &ConsoleService{core: core}
}

// Stats reports the buffer position, dropped count and current epoch.
func (s *ConsoleService) Stats() console.Stats {
	return s.core.Console().Store().Stats()
}

// Range returns up to limit entries with Seq >= fromSeq, oldest first.
func (s *ConsoleService) Range(fromSeq uint64, limit int) []console.Entry {
	entries := s.core.Console().Store().Range(fromSeq, limit)
	if entries == nil {
		entries = []console.Entry{}
	}
	return entries
}

// Clear drops every entry.
func (s *ConsoleService) Clear() {
	s.core.Console().Store().Clear()
	s.core.Bus().Publish(app.TopicConsoleChanged, nil)
}

// Evaluate runs an expression in the attached app and returns the result entry.
func (s *ConsoleService) Evaluate(expression string) (console.Entry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()
	return s.core.Console().Evaluate(ctx, expression)
}

// Properties expands a remote object from an entry of the given epoch.
func (s *ConsoleService) Properties(objectID string, epoch uint64) ([]console.Property, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	props, err := s.core.Console().Properties(ctx, objectID, epoch)
	if props == nil {
		props = []console.Property{}
	}
	return props, err
}
