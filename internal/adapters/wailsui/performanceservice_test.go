package wailsui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/fastrefresh"
	"github.com/Reidevl/rn-lens/internal/perf"
)

func TestPerformanceWithoutAnAttachedApp(t *testing.T) {
	t.Parallel()
	svc := NewPerformanceService(app.New(app.Config{SDKAddr: "off"}), nil)
	if st := svc.Status(); st.Tracing || st.Profiling || st.Snapshotting || st.FastRefresh.Phase != fastrefresh.PhaseOff {
		t.Fatalf("Status = %+v", st)
	}
	if _, err := svc.Screenshot("png", 0); err == nil {
		t.Fatal("Screenshot without an app succeeded")
	}
	if err := svc.StartTrace(nil); err == nil || svc.Status().Tracing {
		t.Fatalf("StartTrace without an app err = %v, tracing %v", err, svc.Status().Tracing)
	}
	for name, save := range map[string]func() (string, error){
		"screenshot": svc.SaveScreenshot, "trace": svc.SaveTrace, "profile": svc.SaveProfile,
	} {
		if _, err := save(); !errors.Is(err, errNothingToSave) {
			t.Fatalf("Save %s before recording err = %v", name, err)
		}
	}
}

func TestSaveTraceWritesAChromeTrace(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trace.json")
	var gotName, gotPattern string
	svc := NewPerformanceService(app.New(app.Config{SDKAddr: "off"}), func(name, _, pattern string) (string, error) {
		gotName, gotPattern = name, pattern
		return path, nil
	})
	svc.trace = &perf.Trace{Events: []json.RawMessage{json.RawMessage(`{"name":"a","ph":"I","ts":1}`)}}

	got, err := svc.SaveTrace()
	if err != nil || got != path {
		t.Fatalf("SaveTrace = %q, %v", got, err)
	}
	if filepath.Ext(gotName) != ".json" || gotPattern != "*.json" {
		t.Fatalf("dialog name %q pattern %q", gotName, gotPattern)
	}
	raw, _ := os.ReadFile(path)
	var file struct {
		TraceEvents []map[string]any `json:"traceEvents"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.TraceEvents) != 1 {
		t.Fatalf("file = %s (%v)", raw, err)
	}
}

func TestSaveCanceledWritesNothing(t *testing.T) {
	t.Parallel()
	svc := NewPerformanceService(app.New(app.Config{SDKAddr: "off"}), func(string, string, string) (string, error) { return "", nil })
	svc.screenshot = &perf.Image{Format: "jpeg", Data: []byte{1}}
	if got, err := svc.SaveScreenshot(); got != "" || err != nil {
		t.Fatalf("SaveScreenshot after cancel = %q, %v", got, err)
	}
	if res, err := svc.SaveHeapSnapshot(); res.Path != "" || err != nil {
		t.Fatalf("SaveHeapSnapshot after cancel = %+v, %v", res, err)
	}
}
