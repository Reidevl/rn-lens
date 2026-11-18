package wailsui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/network"
)

// harTimeout bounds a HAR export, which fetches every body it can.
const harTimeout = 60 * time.Second

// SavePathFunc asks the user where to save a file. It returns "" when the
// user cancels.
type SavePathFunc func(suggestedName string) (string, error)

// NetworkService exposes the captured requests to the frontend. The UI pulls
// Stats then Since(version) after each app.TopicNetworkChanged notice.
type NetworkService struct {
	core     *app.App
	savePath SavePathFunc
}

// NewNetworkService wraps core. savePath shows the native save dialog.
func NewNetworkService(core *app.App, savePath SavePathFunc) *NetworkService {
	return &NetworkService{core: core, savePath: savePath}
}

// Availability reports whether the attached app supports network inspection.
func (s *NetworkService) Availability() network.Availability {
	return s.core.Network().Availability()
}

// Stats reports the store version, size and epoch.
func (s *NetworkService) Stats() network.Stats {
	return s.core.Network().Store().Stats()
}

// Since returns up to limit rows changed after version, in version order.
func (s *NetworkService) Since(version uint64, limit int) []network.Summary {
	return s.core.Network().Store().Since(version, limit)
}

// Request returns one request with headers, timing and initiator.
func (s *NetworkService) Request(seq uint64) (network.Request, error) {
	r, ok := s.core.Network().Store().Get(seq)
	if !ok {
		return network.Request{}, network.ErrNotFound
	}
	return r, nil
}

// ResponseBody fetches the response body of request seq from epoch.
func (s *NetworkService) ResponseBody(seq, epoch uint64) (network.Body, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.Network().ResponseBody(ctx, seq, epoch)
}

// RequestBody returns the body the app sent with request seq from epoch.
func (s *NetworkService) RequestBody(seq, epoch uint64) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.Network().RequestBody(ctx, seq, epoch)
}

// Clear drops every captured request.
func (s *NetworkService) Clear() {
	s.core.Network().Clear()
}

// SaveHAR asks where to save and writes a HAR of every captured request. It
// returns the path written, or "" when the user canceled.
func (s *NetworkService) SaveHAR() (string, error) {
	if s.savePath == nil {
		return "", errors.New("network: no save dialog available")
	}
	name := "lens-" + time.Now().Format("2006-01-02-150405") + ".har"
	path, err := s.savePath(name)
	if err != nil || path == "" {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), harTimeout)
	defer cancel()
	data, err := s.core.Network().ExportHAR(ctx, s.core.Info().Version)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("network: write HAR: %w", err)
	}
	return path, nil
}
