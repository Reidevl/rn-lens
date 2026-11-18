package wailsui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
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

// errInvalidValue rejects a value whose text does not match its type.
var errInvalidValue = errors.New("value does not match its type")

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
	v, err := parseValue(valueType, value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storageActionTimeout)
	defer cancel()
	return s.core.Storage().Set(ctx, deviceKey, storeID, key, v)
}

func parseValue(valueType, value string) (protocol.StorageValue, error) {
	v := protocol.StorageValue{Type: valueType, Value: value}
	switch valueType {
	case protocol.StorageValueString:
	case protocol.StorageValueNumber:
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return v, fmt.Errorf("%w: %q is not a number", errInvalidValue, value)
		}
		v.Value = strconv.FormatFloat(f, 'g', -1, 64)
	case protocol.StorageValueBoolean:
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return v, fmt.Errorf("%w: %q is not true or false", errInvalidValue, value)
		}
		v.Value = strconv.FormatBool(b)
	case protocol.StorageValueBytes:
		if _, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value)); err != nil {
			return v, fmt.Errorf("%w: bytes must be base64", errInvalidValue)
		}
		v.Value = strings.TrimSpace(value)
	default:
		return v, fmt.Errorf("%w: unknown value type %q", storage.ErrInvalidAction, valueType)
	}
	return v, nil
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
	if _, err := s.core.Storage().Stores(deviceKey); err != nil {
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
	if s.openPath == nil {
		return ImportOutcome{}, errors.New("storage: no open dialog available")
	}
	path, err := s.openPath("")
	if err != nil || path == "" {
		return ImportOutcome{}, err
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the user picked this file in the open dialog
	if err != nil {
		return ImportOutcome{}, fmt.Errorf("storage: read import: %w", err)
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
