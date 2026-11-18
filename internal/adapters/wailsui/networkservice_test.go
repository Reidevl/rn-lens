package wailsui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/network"
)

func TestSaveHARWritesTheArchiveWhereTheUserChose(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "session.har")
	var suggested string
	svc := NewNetworkService(app.New(app.Config{SDKAddr: "off", Version: "9.9.9"}), func(name string) (string, error) {
		suggested = name
		return path, nil
	})

	got, err := svc.SaveHAR()
	if err != nil || got != path {
		t.Fatalf("SaveHAR = %q, %v; want %q", got, err, path)
	}
	if filepath.Ext(suggested) != ".har" {
		t.Fatalf("suggested name %q has no .har extension", suggested)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var har network.HAR
	if err := json.Unmarshal(raw, &har); err != nil || har.Log.Creator.Version != "9.9.9" {
		t.Fatalf("written file = %s (%v)", raw, err)
	}
}

func TestSaveHARDoesNothingWhenTheDialogIsCanceled(t *testing.T) {
	t.Parallel()
	svc := NewNetworkService(app.New(app.Config{SDKAddr: "off"}), func(string) (string, error) { return "", nil })
	if got, err := svc.SaveHAR(); err != nil || got != "" {
		t.Fatalf("SaveHAR after cancel = %q, %v; want empty and nil", got, err)
	}
}

func TestRequestReportsMissingRows(t *testing.T) {
	t.Parallel()
	svc := NewNetworkService(app.New(app.Config{SDKAddr: "off"}), nil)
	if _, err := svc.Request(42); !errors.Is(err, network.ErrNotFound) {
		t.Fatalf("Request(42) err = %v, want ErrNotFound", err)
	}
	if rows := svc.Since(0, 0); rows == nil {
		t.Fatal("Since returned nil; the frontend expects an empty array")
	}
}
