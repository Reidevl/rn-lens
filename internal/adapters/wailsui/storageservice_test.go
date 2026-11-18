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
	if err := svc.Set("nobody|1", "async-storage", "k", "number", "not a number"); !errors.Is(err, storage.ErrInvalidValue) {
		t.Fatalf("Set(bad number) err = %v, want ErrInvalidValue", err)
	}
	if err := svc.Set("nobody|1", "async-storage", "k", "bytes", "%%%"); !errors.Is(err, storage.ErrInvalidValue) {
		t.Fatalf("Set(bad base64) err = %v, want ErrInvalidValue", err)
	}
}

func TestStorageExportAndImportNeedAKnownDeviceAndRespectCancel(t *testing.T) {
	t.Parallel()
	asked := false
	ask := func(string) (string, error) { asked = true; return "", nil }
	svc := newStorageService(t, ask, ask)
	if _, err := svc.Export("nobody|1", "async-storage"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Export err = %v", err)
	}
	if _, err := svc.Import("nobody|1", "async-storage"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Import err = %v, want ErrUnknownDevice before any dialog", err)
	}
	if asked {
		t.Fatal("a dialog opened for an unknown device")
	}
}

func TestReadLimitedRefusesLargeFiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(path, make([]byte, 2<<20+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLimited(path, 2<<20); !errors.Is(err, storage.ErrInvalidExport) {
		t.Fatalf("readLimited(over limit) err = %v, want ErrInvalidExport", err)
	}
	if raw, err := readLimited(path, 4<<20); err != nil || len(raw) != 2<<20+1 {
		t.Fatalf("readLimited(under limit) = %d bytes, %v", len(raw), err)
	}
}
