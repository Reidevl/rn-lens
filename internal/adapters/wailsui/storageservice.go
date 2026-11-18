package wailsui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/storage"
)

const (
	storageActionTimeout   = 15 * time.Second
	storageTransferTimeout = 2 * time.Minute
)

// maxImportBytes bounds the export file Import reads.
const maxImportBytes = 64 << 20

// StorageService exposes the app's stores to the frontend. State is pulled
// after each app.TopicStorageChanged notice (ADR-0003); values are read on
// demand.
type StorageService struct {
	core     *app.App
	savePath SavePathFunc
	openPath SavePathFunc
}

// NewStorageService wraps core. savePath and openPath show the native save
// and open dialogs; they return "" when the user cancels.
func NewStorageService(core *app.App, savePath, openPath SavePathFunc) *StorageService {
	return &StorageService{core: core, savePath: savePath, openPath: openPath}
}

// ImportOutcome is what Import did, and with which file.
type ImportOutcome struct {
	Path string `json:"path"`
	storage.ImportResult
}

// Devices lists connected devices that expose storage.
func (s *StorageService) Devices() []storage.Device {
	devs := s.core.Storage().Devices()
	if devs == nil {
		devs = []storage.Device{}
	}
	return devs
}

// Stores returns a device's stores and their keys.
func (s *StorageService) Stores(deviceKey string) ([]protocol.StorageStore, error) {
	stores, err := s.core.Storage().Stores(deviceKey)
	if stores == nil {
		stores = []protocol.StorageStore{}
	}
	return stores, err
}

// Get reads one value from the device.
func (s *StorageService) Get(deviceKey, storeID, key string) (protocol.StorageValue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Get(ctx, deviceKey, storeID, key)
}

// Set writes one value. valueType is string, number, boolean or bytes; value
// is its text (numbers and booleans as typed, bytes as base64).
func (s *StorageService) Set(deviceKey, storeID, key, valueType, value string) error {
	v, err := storage.NormalizeValue(protocol.StorageValue{Type: valueType, Value: value})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Set(ctx, deviceKey, storeID, key, v)
}

// Remove deletes one key.
func (s *StorageService) Remove(deviceKey, storeID, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Remove(ctx, deviceKey, storeID, key)
}

// Clear deletes every key of a store.
func (s *StorageService) Clear(deviceKey, storeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Clear(ctx, deviceKey, storeID)
}

// Refresh asks the device for a fresh snapshot of a store.
func (s *StorageService) Refresh(deviceKey, storeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Refresh(ctx, deviceKey, storeID)
}

// Export asks where to save, then writes every value of a store as JSON. It
// returns the path, or "" when the user canceled.
func (s *StorageService) Export(deviceKey, storeID string) (string, error) {
	if err := s.checkStore(deviceKey, storeID); err != nil {
		return "", err
	}
	if s.savePath == nil {
		return "", errors.New("storage: no save dialog available")
	}
	path, err := s.savePath(fmt.Sprintf("lens-%s-%s.json", strings.ReplaceAll(storeID, ":", "-"), time.Now().Format("2006-01-02-150405")))
	if err != nil || path == "" {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storageTransferTimeout)
	defer cancel()
	file, err := storage.Export(ctx, s.core.Storage(), deviceKey, storeID)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", fmt.Errorf("storage: encode export: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("storage: write export: %w", err)
	}
	return path, nil
}

// Import asks for an export file and writes its entries into a store.
func (s *StorageService) Import(deviceKey, storeID string) (ImportOutcome, error) {
	if err := s.checkStore(deviceKey, storeID); err != nil {
		return ImportOutcome{}, err
	}
	if s.openPath == nil {
		return ImportOutcome{}, errors.New("storage: no open dialog available")
	}
	path, err := s.openPath("")
	if err != nil || path == "" {
		return ImportOutcome{}, err
	}
	raw, err := readLimited(path, maxImportBytes)
	if err != nil {
		return ImportOutcome{}, err
	}
	var file storage.ExportFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return ImportOutcome{}, fmt.Errorf("%w: %w", storage.ErrInvalidExport, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), storageTransferTimeout)
	defer cancel()
	res, err := storage.Import(ctx, s.core.Storage(), deviceKey, storeID, file)
	return ImportOutcome{Path: path, ImportResult: res}, err
}

// checkStore fails before any dialog opens when the device or store is gone.
func (s *StorageService) checkStore(deviceKey, storeID string) error {
	stores, err := s.core.Storage().Stores(deviceKey)
	if err != nil {
		return err
	}
	for _, st := range stores {
		if st.ID == storeID {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", storage.ErrUnknownStore, storeID)
}

// readLimited reads a file the user picked, refusing ones above limit bytes.
func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the user picked this file in the open dialog
	if err != nil {
		return nil, fmt.Errorf("storage: read import: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("storage: read import: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: the file is larger than %d MiB", storage.ErrInvalidExport, limit>>20)
	}
	return raw, nil
}
