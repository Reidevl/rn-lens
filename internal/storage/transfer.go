package storage

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
)

// ExportFormat marks a JSON file as a Lens storage export.
const ExportFormat = "lens-storage-export"

// exportVersion is the only export layout Import reads.
const exportVersion = 1

// ErrInvalidExport means a file is not a Lens storage export Import can read.
var ErrInvalidExport = errors.New("storage: not a Lens storage export")

// ExportFile is the JSON layout of an exported store. Values keep their wire
// form, so bytes stay base64 and truncated values carry only their size.
type ExportFile struct {
	Format     string                           `json:"format"`
	Version    int                              `json:"version"`
	StoreID    string                           `json:"storeId"`
	Kind       string                           `json:"kind"`
	Name       string                           `json:"name"`
	ExportedAt time.Time                        `json:"exportedAt"`
	Entries    map[string]protocol.StorageValue `json:"entries"`
	// Failed lists keys the device could not read, with the reason.
	Failed map[string]string `json:"failed,omitempty"`
}

// ImportResult says what an import wrote and what it left out, and why.
type ImportResult struct {
	Written int               `json:"written"`
	Skipped map[string]string `json:"skipped"`
}

// Export reads every key of a store from the device. Keys the device cannot
// read are listed in Failed instead of failing the export.
func Export(ctx context.Context, svc *Service, deviceKey, storeID string) (ExportFile, error) {
	stores, err := svc.Stores(deviceKey)
	if err != nil {
		return ExportFile{}, err
	}
	var store *protocol.StorageStore
	for i := range stores {
		if stores[i].ID == storeID {
			store = &stores[i]
		}
	}
	if store == nil {
		return ExportFile{}, fmt.Errorf("%w: %s", ErrUnknownStore, storeID)
	}
	file := ExportFile{
		Format: ExportFormat, Version: exportVersion, StoreID: store.ID, Kind: store.Kind, Name: store.Name,
		ExportedAt: time.Now().UTC(), Entries: make(map[string]protocol.StorageValue, len(store.Keys)),
	}
	for _, k := range store.Keys {
		if err := ctx.Err(); err != nil {
			return ExportFile{}, fmt.Errorf("storage: export: %w", err)
		}
		v, err := svc.Get(ctx, deviceKey, storeID, k.Key)
		if errors.Is(err, ErrDisconnected) || errors.Is(err, ErrUnknownDevice) {
			return ExportFile{}, fmt.Errorf("storage: export: %w", err)
		}
		if err != nil {
			if file.Failed == nil {
				file.Failed = make(map[string]string)
			}
			file.Failed[k.Key] = err.Error()
			continue
		}
		file.Entries[k.Key] = v
	}
	return file, nil
}

// Import writes every entry of an export into a store, one SET per key.
// Truncated entries (their value was not exported) and entries the device
// refuses are reported in Skipped; the rest are written.
func Import(ctx context.Context, svc *Service, deviceKey, storeID string, file ExportFile) (ImportResult, error) {
	if file.Format != ExportFormat || file.Version != exportVersion {
		return ImportResult{}, fmt.Errorf("%w (format %q, version %d)", ErrInvalidExport, file.Format, file.Version)
	}
	stores, err := svc.Stores(deviceKey)
	if err != nil {
		return ImportResult{}, err
	}
	if !slices.ContainsFunc(stores, func(st protocol.StorageStore) bool { return st.ID == storeID }) {
		return ImportResult{}, fmt.Errorf("%w: %s", ErrUnknownStore, storeID)
	}
	res := ImportResult{Skipped: map[string]string{}}
	for k, v := range file.Entries {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("storage: import: %w", err)
		}
		if v.Truncated {
			res.Skipped[k] = "the export holds only the size of this value"
			continue
		}
		v, err := NormalizeValue(v)
		if err != nil {
			res.Skipped[k] = err.Error()
			continue
		}
		if err := svc.Set(ctx, deviceKey, storeID, k, v); err != nil {
			// The device going away ends the import; a refused key does not.
			if errors.Is(err, ErrDisconnected) || errors.Is(err, ErrUnknownDevice) || errors.Is(err, ErrUnknownStore) {
				return res, fmt.Errorf("storage: import: %w", err)
			}
			res.Skipped[k] = err.Error()
			continue
		}
		res.Written++
	}
	return res, nil
}
