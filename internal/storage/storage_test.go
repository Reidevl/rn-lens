package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/storage"
)

// fakeDevice records the frames Lens sends and can answer storage actions.
type fakeDevice struct {
	mu     sync.Mutex
	sent   []protocol.StorageAction
	answer func(protocol.StorageAction) *protocol.StorageResult
	svc    *storage.Service
	key    string
	sessID string
}

func (f *fakeDevice) Send(typ, id string, payload any) error {
	if typ != protocol.TypeStorageAction {
		return nil
	}
	a, ok := payload.(protocol.StorageAction)
	if !ok {
		return errors.New("unexpected payload")
	}
	f.mu.Lock()
	f.sent = append(f.sent, a)
	answer := f.answer
	f.mu.Unlock()
	if answer == nil {
		return nil
	}
	if res := answer(a); res != nil {
		go func() { _ = f.svc.Message(f.key, f.sessID, frame(protocol.TypeStorageResult, id, *res)) }()
	}
	return nil
}

// frame builds a decoded message as sdkws hands it over.
func frame(typ, id string, payload any) protocol.Message {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return protocol.Message{V: protocol.Version, Type: typ, ID: id, Payload: raw}
}

func hello(features ...string) protocol.Hello {
	return protocol.Hello{
		ProtocolVersion: protocol.Version, SDKVersion: "0.1.0", Features: features,
		App:    protocol.AppInfo{ID: "dev.rnlens.example", Name: "Lens Example", Platform: "ios"},
		Device: protocol.DeviceInfo{ID: "d1", Name: "iPhone"},
	}
}

func connected(t *testing.T) (*storage.Service, *fakeDevice, *atomic.Int64) {
	t.Helper()
	var notified atomic.Int64
	svc := storage.NewService(func() { notified.Add(1) })
	dev := &fakeDevice{svc: svc, key: "dev.rnlens.example|d1", sessID: "s1"}
	svc.Connect("s1", hello(protocol.FeatureStorage), dev)
	return svc, dev, &notified
}

var snapshot = protocol.StorageSnapshot{Stores: []protocol.StorageStore{
	{ID: "async-storage", Kind: protocol.StorageKindAsyncStorage, Name: "AsyncStorage", Keys: []protocol.StorageKey{{Key: "theme", Size: 6}, {Key: "session", Size: 40}}},
	{ID: "mmkv:default", Kind: protocol.StorageKindMMKV, Name: "default", Live: true, Keys: []protocol.StorageKey{{Key: "launches", Size: 2}}},
}}

func keysOf(t *testing.T, svc *storage.Service, storeID string) []string {
	t.Helper()
	stores, err := svc.Stores("dev.rnlens.example|d1")
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range stores {
		if st.ID == storeID {
			out := make([]string, len(st.Keys))
			for i, k := range st.Keys {
				out[i] = k.Key
			}
			return out
		}
	}
	t.Fatalf("store %s not found in %+v", storeID, stores)
	return nil
}

func TestOnlyDevicesAnnouncingStorageAreListed(t *testing.T) {
	t.Parallel()
	svc := storage.NewService(nil)
	svc.Connect("q", protocol.Hello{App: protocol.AppInfo{ID: "a"}, Device: protocol.DeviceInfo{ID: "1"}, Features: []string{protocol.FeatureQueries}}, &fakeDevice{})
	svc.Connect("s", protocol.Hello{App: protocol.AppInfo{ID: "b"}, Device: protocol.DeviceInfo{ID: "1"}, Features: []string{protocol.FeatureStorage}}, &fakeDevice{})
	devices := svc.Devices()
	if len(devices) != 1 || devices[0].Key != "b|1" {
		t.Fatalf("devices = %+v", devices)
	}
	if _, err := svc.Stores("a|1"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Stores(queries-only device) err = %v", err)
	}
}

func TestSnapshotAndLiveEventsMirrorTheStores(t *testing.T) {
	t.Parallel()
	svc, _, notified := connected(t)
	key := "dev.rnlens.example|d1"
	if err := svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot)); err != nil {
		t.Fatal(err)
	}
	if got := keysOf(t, svc, "async-storage"); len(got) != 2 || got[0] != "session" || got[1] != "theme" {
		t.Fatalf("keys = %v, want sorted", got)
	}
	before := notified.Load()

	for _, ev := range []protocol.StorageEvent{
		{StoreID: "mmkv:default", Kind: protocol.StorageEventSet, Key: "token", Size: 32},
		{StoreID: "mmkv:default", Kind: protocol.StorageEventRemoved, Key: "launches"},
	} {
		if err := svc.Message(key, "s1", frame(protocol.TypeStorageEvent, "", ev)); err != nil {
			t.Fatal(err)
		}
	}
	if got := keysOf(t, svc, "mmkv:default"); len(got) != 1 || got[0] != "token" {
		t.Fatalf("mmkv keys = %v", got)
	}
	if notified.Load() <= before {
		t.Fatal("events did not notify")
	}
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageEvent, "", protocol.StorageEvent{StoreID: "mmkv:default", Kind: protocol.StorageEventCleared}))
	if got := keysOf(t, svc, "mmkv:default"); len(got) != 0 {
		t.Fatalf("keys after clear = %v", got)
	}

	// A snapshot for one store replaces only that store.
	one := protocol.StorageSnapshot{StoreID: "async-storage", Stores: []protocol.StorageStore{{ID: "async-storage", Kind: protocol.StorageKindAsyncStorage, Name: "AsyncStorage", Keys: []protocol.StorageKey{{Key: "fresh"}}}}}
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", one))
	stores, _ := svc.Stores(key)
	if len(stores) != 2 || keysOf(t, svc, "async-storage")[0] != "fresh" {
		t.Fatalf("stores after a one-store snapshot = %+v", stores)
	}
}

func TestFramesFromAReplacedSessionAreIgnored(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	svc.Connect("s2", hello(protocol.FeatureStorage), &fakeDevice{})
	err := svc.Message("dev.rnlens.example|d1", "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))
	if !errors.Is(err, storage.ErrStaleSession) {
		t.Fatalf("err = %v, want ErrStaleSession", err)
	}
	if stores, _ := svc.Stores("dev.rnlens.example|d1"); len(stores) != 0 {
		t.Fatalf("stale snapshot landed: %+v", stores)
	}
}

func TestGetAndSetRoundTripThroughTheDevice(t *testing.T) {
	t.Parallel()
	svc, dev, _ := connected(t)
	key := "dev.rnlens.example|d1"
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))
	dev.answer = func(a protocol.StorageAction) *protocol.StorageResult {
		switch a.Type {
		case protocol.StorageGet:
			return &protocol.StorageResult{ID: a.ID, OK: true, Value: &protocol.StorageValue{Type: protocol.StorageValueString, Value: "dark"}}
		case protocol.StorageSet:
			return &protocol.StorageResult{ID: a.ID, OK: true}
		default:
			return &protocol.StorageResult{ID: a.ID, OK: false, Error: "key is excluded by the app"}
		}
	}
	ctx := context.Background()

	v, err := svc.Get(ctx, key, "async-storage", "theme")
	if err != nil || v.Value != "dark" {
		t.Fatalf("Get = %+v, %v", v, err)
	}
	if err := svc.Set(ctx, key, "async-storage", "theme", protocol.StorageValue{Type: protocol.StorageValueString, Value: "light"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := svc.Remove(ctx, key, "async-storage", "secret"); !errors.Is(err, storage.ErrActionFailed) {
		t.Fatalf("Remove err = %v, want ErrActionFailed", err)
	}
	dev.mu.Lock()
	defer dev.mu.Unlock()
	if len(dev.sent) != 3 || dev.sent[1].Value == nil || dev.sent[1].Value.Value != "light" || dev.sent[0].ID == dev.sent[1].ID {
		t.Fatalf("sent = %+v", dev.sent)
	}
	if dev.sent[0].ID[0] != 's' {
		t.Fatalf("id %q does not use the storage prefix", dev.sent[0].ID)
	}
}

func TestActionsAreValidatedLocally(t *testing.T) {
	t.Parallel()
	svc, dev, _ := connected(t)
	key := "dev.rnlens.example|d1"
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))
	ctx := context.Background()
	if _, err := svc.Get(ctx, key, "nope", "theme"); !errors.Is(err, storage.ErrUnknownStore) {
		t.Fatalf("Get(unknown store) err = %v", err)
	}
	if err := svc.Set(ctx, key, "async-storage", "", protocol.StorageValue{Type: "string"}); !errors.Is(err, storage.ErrInvalidAction) {
		t.Fatalf("Set(empty key) err = %v", err)
	}
	if err := svc.Set(ctx, key, "async-storage", "k", protocol.StorageValue{Type: "date"}); !errors.Is(err, storage.ErrInvalidAction) {
		t.Fatalf("Set(bad type) err = %v", err)
	}
	if _, err := svc.Get(ctx, "nobody|1", "async-storage", "k"); !errors.Is(err, storage.ErrUnknownDevice) {
		t.Fatalf("Get(unknown device) err = %v", err)
	}
	if len(dev.sent) != 0 {
		t.Fatalf("invalid actions reached the device: %+v", dev.sent)
	}
}

func TestPendingActionsEndWhenTheDeviceLeavesOrTimesOut(t *testing.T) {
	t.Parallel()
	svc, dev, _ := connected(t)
	key := "dev.rnlens.example|d1"
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := svc.Get(ctx, key, "async-storage", "theme"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get without an answer err = %v, want the deadline", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.Get(context.Background(), key, "async-storage", "theme")
		done <- err
	}()
	// Disconnect only once the second action is on the wire.
	deadline := time.Now().Add(2 * time.Second)
	for sent := 0; sent < 2; {
		if time.Now().After(deadline) {
			t.Fatal("the second action never reached the device")
		}
		dev.mu.Lock()
		sent = len(dev.sent)
		dev.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	svc.Disconnect(key, "s1")
	select {
	case err := <-done:
		if !errors.Is(err, storage.ErrDisconnected) {
			t.Fatalf("err = %v, want ErrDisconnected", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Get did not end when the device disconnected")
	}
	if len(svc.Devices()) != 0 {
		t.Fatal("device still listed after disconnect")
	}
}

func TestAOneStoreSnapshotWithoutTheStoreRemovesIt(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	key := "dev.rnlens.example|d1"
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", protocol.StorageSnapshot{StoreID: "mmkv:default", Stores: []protocol.StorageStore{}}))
	stores, _ := svc.Stores(key)
	if len(stores) != 1 || stores[0].ID != "async-storage" {
		t.Fatalf("stores = %+v, want only async-storage", stores)
	}
	// A one-store snapshot ignores stores with another id.
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", protocol.StorageSnapshot{StoreID: "async-storage", Stores: []protocol.StorageStore{
		{ID: "async-storage", Kind: protocol.StorageKindAsyncStorage, Name: "AsyncStorage"},
		{ID: "intruder", Kind: protocol.StorageKindMMKV, Name: "intruder"},
	}}))
	if stores, _ := svc.Stores(key); len(stores) != 1 {
		t.Fatalf("stores = %+v, want the other id ignored", stores)
	}
}

func TestAOneStoreSnapshotKeepsTheStoreInPlace(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	key := "dev.rnlens.example|d1"
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", snapshot))
	_ = svc.Message(key, "s1", frame(protocol.TypeStorageSnapshot, "", protocol.StorageSnapshot{StoreID: "async-storage", Stores: []protocol.StorageStore{
		{ID: "async-storage", Kind: protocol.StorageKindAsyncStorage, Name: "AsyncStorage", Keys: []protocol.StorageKey{{Key: "fresh"}}},
	}}))
	stores, _ := svc.Stores(key)
	if len(stores) != 2 || stores[0].ID != "async-storage" || stores[1].ID != "mmkv:default" {
		t.Fatalf("store order = %v, want the refreshed store to keep its place", []string{stores[0].ID, stores[1].ID})
	}
}
