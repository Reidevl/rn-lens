package wailsui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/storage"
)

func newStorageService(t *testing.T, save, open SavePathFunc) *StorageService {
	t.Helper()
	return NewStorageService(app.New(app.Config{SDKAddr: "off"}), save, open)
}

func TestStorageServiceListsNothingWithoutDevices(t *testing.T) {
	t.Parallel()
	svc := newStorageService(t, nil, nil)
	if devs := svc.Devices(); devs == nil || len(devs) != 0 {
		t.Fatalf("Devices = %#v, want an empty array", devs)
	}
	if _, err := svc.Stores("nobody|1"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Stores err = %v", err)
	}
}

func TestStorageSetValidatesTheValueBeforeTheDevice(t *testing.T) {
	t.Parallel()
	svc := newStorageService(t, nil, nil)
	if err := svc.Set("nobody|1", "async-storage", "k", "date", "x"); !errors.Is(err, storage.ErrInvalidAction) {
		t.Fatalf("Set(bad type) err = %v, want ErrInvalidAction", err)
	}
	if err := svc.Set("nobody|1", "async-storage", "k", "number", "not a number"); !errors.Is(err, errInvalidValue) {
		t.Fatalf("Set(bad number) err = %v, want errInvalidValue", err)
	}
	if err := svc.Set("nobody|1", "async-storage", "k", "bytes", "%%%"); !errors.Is(err, errInvalidValue) {
		t.Fatalf("Set(bad base64) err = %v, want errInvalidValue", err)
	}
}

func TestStorageExportAndImportNeedAKnownDeviceAndRespectCancel(t *testing.T) {
	t.Parallel()
	asked := false
	svc := newStorageService(t, func(string) (string, error) { asked = true; return "", nil }, func(string) (string, error) { return "", nil })
	if _, err := svc.Export("nobody|1", "async-storage"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Export err = %v", err)
	}
	if asked {
		t.Fatal("the save dialog opened for an unknown device")
	}
	res, err := svc.Import("nobody|1", "async-storage")
	if err != nil || res.Path != "" {
		t.Fatalf("Import after cancel = %+v, %v; want nothing done", res, err)
	}
}

func TestStorageImportRejectsFilesThatAreNotExports(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	if err := os.WriteFile(path, []byte(`{"hello":"world"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newStorageService(t, nil, func(string) (string, error) { return path, nil })
	if _, err := svc.Import("nobody|1", "async-storage"); !errors.Is(err, storage.ErrInvalidExport) {
		t.Fatalf("Import(non-export) err = %v, want ErrInvalidExport", err)
	}
}
