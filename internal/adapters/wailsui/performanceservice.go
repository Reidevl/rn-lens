package wailsui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/fastrefresh"
	"github.com/Reidevl/rn-lens/internal/perf"
)

const (
	// traceStopTimeout covers Tracing.end plus every chunk the app sends.
	traceStopTimeout = 60 * time.Second
	// heapTimeout covers a full Hermes heap snapshot of a large app.
	heapTimeout = 5 * time.Minute
)

// SaveAsFunc asks the user where to save a file, offering one filter
// (label, pattern such as "*.json"). It returns "" when the user cancels.
type SaveAsFunc func(suggestedName, label, pattern string) (string, error)

// errNothingToSave means the panel asked to save a result it never got.
var errNothingToSave = errors.New("performance: nothing to save yet")

// PerformanceStatus is what the panel and the status bar show.
type PerformanceStatus struct {
	perf.Status
	FastRefresh fastrefresh.State `json:"fastRefresh"`
}

// Screenshot is a captured image for the panel preview.
type Screenshot struct {
	Format string `json:"format"`
	// Data is the image, base64-encoded.
	Data  string `json:"data"`
	Bytes int    `json:"bytes"`
}

// HeapSnapshotResult reports a saved heap snapshot.
type HeapSnapshotResult struct {
	// Path is "" when the user canceled the dialog.
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// PerformanceService exposes screenshots, traces, CPU profiles and heap
// snapshots. The UI pulls Status after each app.TopicPerformanceChanged.
type PerformanceService struct {
	core   *app.App
	saveAs SaveAsFunc

	mu         sync.Mutex
	screenshot *perf.Image
	trace      *perf.Trace
	profile    []byte
}

// NewPerformanceService wraps core. saveAs shows the native save dialog.
func NewPerformanceService(core *app.App, saveAs SaveAsFunc) *PerformanceService {
	return &PerformanceService{core: core, saveAs: saveAs}
}

// Status reports the recordings in progress and the last Fast Refresh.
func (s *PerformanceService) Status() PerformanceStatus {
	return PerformanceStatus{Status: s.core.Performance().Status(), FastRefresh: s.core.FastRefresh()}
}

// TraceCategories lists the categories recorded by default.
func (s *PerformanceService) TraceCategories() []string {
	return append([]string(nil), perf.DefaultTraceCategories...)
}

// Screenshot captures the app's screen as "png" or "jpeg" (quality 1-100).
func (s *PerformanceService) Screenshot(format string, quality int) (Screenshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	img, err := s.core.Performance().Screenshot(ctx, format, quality)
	if err != nil {
		return Screenshot{}, err
	}
	s.mu.Lock()
	s.screenshot = &img
	s.mu.Unlock()
	return Screenshot{Format: img.Format, Data: base64.StdEncoding.EncodeToString(img.Data), Bytes: len(img.Data)}, nil
}

// SaveScreenshot saves the last screenshot. It returns the path, or "" when
// the user canceled.
func (s *PerformanceService) SaveScreenshot() (string, error) {
	s.mu.Lock()
	img := s.screenshot
	s.mu.Unlock()
	if img == nil {
		return "", errNothingToSave
	}
	ext := "png"
	if img.Format == "jpeg" {
		ext = "jpg"
	}
	return s.save("lens-screenshot-"+stamp()+"."+ext, "Image", "*."+ext, img.Data)
}

// StartTrace starts recording a trace; empty categories use the defaults.
func (s *PerformanceService) StartTrace(categories []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.Performance().StartTrace(ctx, categories)
}

// StopTrace ends the recording and keeps the trace for SaveTrace.
func (s *PerformanceService) StopTrace() (perf.TraceSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), traceStopTimeout)
	defer cancel()
	trace, err := s.core.Performance().StopTrace(ctx)
	if err != nil {
		return perf.TraceSummary{}, err
	}
	s.mu.Lock()
	s.trace = &trace
	s.mu.Unlock()
	return trace.Summarize(), nil
}

// SaveTrace saves the last trace as Chrome JSON, which Perfetto UI and
// Chrome DevTools open.
func (s *PerformanceService) SaveTrace() (string, error) {
	s.mu.Lock()
	trace := s.trace
	s.mu.Unlock()
	if trace == nil {
		return "", errNothingToSave
	}
	var buf bytes.Buffer
	if err := perf.WriteTrace(&buf, *trace); err != nil {
		return "", err
	}
	return s.save("lens-trace-"+stamp()+".json", "Chrome trace", "*.json", buf.Bytes())
}

// StartProfile starts the JavaScript sampling profiler.
func (s *PerformanceService) StartProfile() error {
	ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
	defer cancel()
	return s.core.Performance().StartProfile(ctx)
}

// StopProfile stops the profiler and keeps the profile for SaveProfile.
func (s *PerformanceService) StopProfile() (perf.ProfileSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), traceStopTimeout)
	defer cancel()
	profile, err := s.core.Performance().StopProfile(ctx)
	if err != nil {
		return perf.ProfileSummary{}, err
	}
	s.mu.Lock()
	s.profile = profile
	s.mu.Unlock()
	return perf.SummarizeProfile(profile), nil
}

// SaveProfile saves the last CPU profile as a .cpuprofile file.
func (s *PerformanceService) SaveProfile() (string, error) {
	s.mu.Lock()
	profile := s.profile
	s.mu.Unlock()
	if profile == nil {
		return "", errNothingToSave
	}
	return s.save("lens-"+stamp()+".cpuprofile", "CPU profile", "*.cpuprofile", profile)
}

// SaveHeapSnapshot asks where to save, then streams a heap snapshot there.
// It writes a temporary file next to the chosen one and renames it on
// success, so a failed snapshot leaves an existing file untouched.
func (s *PerformanceService) SaveHeapSnapshot() (HeapSnapshotResult, error) {
	path, err := s.ask("lens-"+stamp()+".heapsnapshot", "Heap snapshot", "*.heapsnapshot")
	if err != nil || path == "" {
		return HeapSnapshotResult{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".lens-*.heapsnapshot")
	if err != nil {
		return HeapSnapshotResult{}, fmt.Errorf("performance: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // a no-op after the rename
	// Chunks are small; buffering keeps each event's disk work cheap.
	w := bufio.NewWriterSize(f, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), heapTimeout)
	defer cancel()
	n, err := s.core.Performance().HeapSnapshot(ctx, w)
	if err == nil {
		err = w.Flush()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return HeapSnapshotResult{}, fmt.Errorf("performance: %w", err)
	}
	return HeapSnapshotResult{Path: path, Bytes: n}, nil
}

func (s *PerformanceService) ask(name, label, pattern string) (string, error) {
	if s.saveAs == nil {
		return "", errors.New("performance: no save dialog available")
	}
	return s.saveAs(name, label, pattern)
}

func (s *PerformanceService) save(name, label, pattern string, data []byte) (string, error) {
	path, err := s.ask(name, label, pattern)
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("performance: %w", err)
	}
	return path, nil
}

func stamp() string { return time.Now().Format("2006-01-02-150405") }
