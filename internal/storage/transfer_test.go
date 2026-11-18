package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/storage"
)

// memoryDevice answers GET/SET from a map, like an app's store would.
func memoryDevice(t *testing.T, values map[string]protocol.StorageValue) (*storage.Service, string) {
	t.Helper()
	svc, dev, _ := connected(t)
	key := "dev.rnlens.example|d1"
	keys := make([]protocol.StorageKey, 0, len(values))
	for k := range values {
		keys = append(keys, protocol.StorageKey{Key: k})
	}
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", protocol.StorageSnapshot{Stores: []protocol.StorageStore{
		{ID: "async-storage", Kind: protocol.StorageKindAsyncStorage, Name: "AsyncStorage", Keys: keys},
	}}))
	dev.answer = func(a protocol.StorageAction) *protocol.StorageResult {
		switch a.Type {
		case protocol.StorageGet:
			v, ok := values[a.Key]
			if !ok {
				return &protocol.StorageResult{ID: a.ID, OK: false, Error: "not set"}
			}
			return &protocol.StorageResult{ID: a.ID, OK: true, Value: &v}
		case protocol.StorageSet:
			if a.Key == "locked" {
				return &protocol.StorageResult{ID: a.ID, OK: false, Error: "key is excluded by the app"}
			}
			values[a.Key] = *a.Value
			return &protocol.StorageResult{ID: a.ID, OK: true}
		}
		return &protocol.StorageResult{ID: a.ID, OK: true}
	}
	return svc, key
}

func TestExportReadsEveryKeyOfAStore(t *testing.T) {
	t.Parallel()
	svc, key := memoryDevice(t, map[string]protocol.StorageValue{
		"theme": {Type: protocol.StorageValueString, Value: "dark"},
		"big":   {Type: protocol.StorageValueString, Truncated: true, Size: 2 << 20},
	})
	file, err := storage.Export(context.Background(), svc, key, "async-storage")
	if err != nil {
		t.Fatal(err)
	}
	if file.Format != storage.ExportFormat || file.StoreID != "async-storage" || file.Kind != protocol.StorageKindAsyncStorage || file.ExportedAt.IsZero() {
		t.Fatalf("header = %+v", file)
	}
	if file.Entries["theme"].Value != "dark" || !file.Entries["big"].Truncated || len(file.Entries) != 2 {
		t.Fatalf("entries = %+v", file.Entries)
	}
	raw, _ := json.Marshal(file)
	var back storage.ExportFile
	if err := json.Unmarshal(raw, &back); err != nil || back.Entries["theme"].Value != "dark" {
		t.Fatalf("round trip = %s (%v)", raw, err)
	}
}

func TestImportWritesEntriesAndReportsWhatItSkipped(t *testing.T) {
	t.Parallel()
	values := map[string]protocol.StorageValue{}
	svc, key := memoryDevice(t, values)
	file := storage.ExportFile{Format: storage.ExportFormat, Version: 1, Entries: map[string]protocol.StorageValue{
		"theme":  {Type: protocol.StorageValueString, Value: "light"},
		"big":    {Type: protocol.StorageValueString, Truncated: true, Size: 9},
		"locked": {Type: protocol.StorageValueString, Value: "x"},
	}}
	res, err := storage.Import(context.Background(), svc, key, "async-storage", file)
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 1 || values["theme"].Value != "light" {
		t.Fatalf("result = %+v values = %+v", res, values)
	}
	if len(res.Skipped) != 2 || res.Skipped["big"] == "" || res.Skipped["locked"] == "" {
		t.Fatalf("skipped = %+v, want the truncated and the refused entries", res.Skipped)
	}
}

func TestImportRefusesFilesThatAreNotLensExports(t *testing.T) {
	t.Parallel()
	svc, key := memoryDevice(t, map[string]protocol.StorageValue{})
	for _, f := range []storage.ExportFile{
		{Format: "something-else", Version: 1, Entries: map[string]protocol.StorageValue{"a": {Type: "string"}}},
		{Format: storage.ExportFormat, Version: 2, Entries: map[string]protocol.StorageValue{"a": {Type: "string"}}},
	} {
		if _, err := storage.Import(context.Background(), svc, key, "async-storage", f); !errors.Is(err, storage.ErrInvalidExport) {
			t.Fatalf("Import(%+v) err = %v, want ErrInvalidExport", f, err)
		}
	}
}
