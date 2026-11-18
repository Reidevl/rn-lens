package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// TestE2E_StorageEditReachesTheApp edits an AsyncStorage key from Lens and
// reads it back inside the app with getItem (the M5 acceptance check). It
// needs the example app running with its SDK pointed at this test's SDK
// server: LENS_E2E_METRO for Metro and LENS_E2E_SDK_ADDR (for example
// ":7421", with EXPO_PUBLIC_LENS_URL=ws://127.0.0.1:7421/lens in the app).
func TestE2E_StorageEditReachesTheApp(t *testing.T) {
	endpoint, sdkAddr := os.Getenv("LENS_E2E_METRO"), os.Getenv("LENS_E2E_SDK_ADDR")
	if endpoint == "" || sdkAddr == "" || testing.Short() {
		t.Skip("set LENS_E2E_METRO and LENS_E2E_SDK_ADDR with the example app running to enable")
	}
	a := app.New(app.Config{MetroEndpoints: []string{endpoint}, PollInterval: 200 * time.Millisecond, SDKAddr: sdkAddr})
	storageCh, cancel := a.Bus().Subscribe(app.TopicStorageChanged, 64)
	defer cancel()
	a.Start()
	t.Cleanup(a.Stop)
	ctx, cancelCtx := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelCtx()

	// The SDK reconnects on its own; wait for a device with an AsyncStorage store.
	var deviceKey string
	for deviceKey == "" {
		for _, d := range a.Storage().Devices() {
			if stores, _ := a.Storage().Stores(d.Key); hasStore(stores, "async-storage") {
				deviceKey = d.Key
			}
		}
		if deviceKey != "" {
			break
		}
		select {
		case <-storageCh:
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("no storage device connected; is EXPO_PUBLIC_LENS_URL pointing at LENS_E2E_SDK_ADDR?")
		}
	}
	stores, _ := a.Storage().Stores(deviceKey)
	for _, st := range stores {
		t.Logf("store %s (%s): %d keys", st.ID, st.Kind, len(st.Keys))
	}

	want := "edited in Lens " + time.Now().Format(time.RFC3339Nano)
	if err := a.Storage().Set(ctx, deviceKey, "async-storage", "lens-e2e", protocol.StorageValue{Type: protocol.StorageValueString, Value: want}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := a.Storage().Get(ctx, deviceKey, "async-storage", "lens-e2e")
	if err != nil || got.Value != want {
		t.Fatalf("Get after Set = %+v, %v", got, err)
	}

	// Read it back the way the app would, through the debugger.
	a.RefreshTargets(ctx)
	var target targets.Target
	for _, c := range a.Targets() {
		if c.Reloadable {
			target = c
		}
	}
	if target.ID == "" {
		t.Fatal("no debug target to read the value back")
	}
	if err := a.Attach(target.ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for a.SessionSnapshot().State != targets.StateAttached {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("did not attach")
		}
	}
	expr := "globalThis.__lensExample.AsyncStorage.getItem('lens-e2e').then(v => console.log('lens-e2e-storage', v))"
	if _, err := a.Console().Evaluate(ctx, expr); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Hermes replays earlier console lines on attach, so match this run's value.
	line := "lens-e2e-storage " + want
	for !consoleHas(a, line) {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("the app never logged %q", line)
		}
	}
	t.Logf("app read back: %s", line)
	if err := a.Storage().Remove(ctx, deviceKey, "async-storage", "lens-e2e"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	checkLiveMMKV(ctx, t, a, deviceKey)
}

func hasStore(stores []protocol.StorageStore, id string) bool {
	for _, s := range stores {
		if s.ID == id {
			return true
		}
	}
	return false
}

// checkLiveMMKV writes and deletes a key in the example's MMKV store and waits
// for each change to arrive as a live storage.event (react-native-mmkv v4).
func checkLiveMMKV(ctx context.Context, t *testing.T, a *app.App, deviceKey string) {
	t.Helper()
	const store, key = "mmkv:lens-example", "lens-e2e-live"
	stores, _ := a.Storage().Stores(deviceKey)
	if !hasStore(stores, store) {
		t.Log("no MMKV store (Expo Go?); skipping the live check")
		return
	}
	listed := func() bool {
		stores, _ := a.Storage().Stores(deviceKey)
		for _, st := range stores {
			if st.ID == store {
				for _, k := range st.Keys {
					if k.Key == key {
						return true
					}
				}
			}
		}
		return false
	}
	waitFor := func(what string, want bool) {
		for listed() != want {
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				t.Fatalf("MMKV %s did not arrive live", what)
			}
		}
	}
	if err := a.Storage().Set(ctx, deviceKey, store, key, protocol.StorageValue{Type: protocol.StorageValueNumber, Value: "7"}); err != nil {
		t.Fatalf("MMKV Set: %v", err)
	}
	waitFor("set", true)
	if v, err := a.Storage().Get(ctx, deviceKey, store, key); err != nil || v.Type != protocol.StorageValueNumber || v.Value != "7" {
		t.Fatalf("MMKV Get = %+v, %v", v, err)
	}
	if err := a.Storage().Remove(ctx, deviceKey, store, key); err != nil {
		t.Fatalf("MMKV Remove: %v", err)
	}
	waitFor("removal", false)
	t.Log("MMKV set, typed get and remove arrived live")
}

func consoleHas(a *app.App, text string) bool {
	for _, e := range a.Console().Store().Range(1, 0) {
		if e.Text == text {
			return true
		}
	}
	return false
}
