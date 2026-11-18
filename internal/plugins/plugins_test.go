package plugins_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/plugins"
	"github.com/Reidevl/rn-lens/internal/protocol"
)

const key = "dev.rnlens.example|d1"

type fakeDevice struct {
	mu     sync.Mutex
	sent   []protocol.PluginAction
	answer func(protocol.PluginAction) *protocol.PluginResult
	svc    *plugins.Service
}

func (f *fakeDevice) Send(typ, id string, payload any) error {
	if typ != protocol.TypePluginAction {
		return nil
	}
	a, ok := payload.(protocol.PluginAction)
	if !ok || a.ID != id {
		return errors.New("unexpected payload")
	}
	f.mu.Lock()
	f.sent = append(f.sent, a)
	answer := f.answer
	f.mu.Unlock()
	if answer != nil {
		if res := answer(a); res != nil {
			go func() { _ = f.svc.Message(key, "s1", frame(protocol.TypePluginResult, id, *res)) }()
		}
	}
	return nil
}

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

func connected(t *testing.T) (*plugins.Service, *fakeDevice, *atomic.Int64) {
	t.Helper()
	var notified atomic.Int64
	svc := plugins.NewService(func() { notified.Add(1) })
	dev := &fakeDevice{svc: svc}
	svc.Connect("s1", hello(protocol.FeaturePlugins), dev)
	return svc, dev, &notified
}

func register(t *testing.T, svc *plugins.Service, id, name string) {
	t.Helper()
	if err := svc.Message(key, "s1", frame(protocol.TypePluginRegister, "", protocol.PluginRegister{ID: id, Name: name})); err != nil {
		t.Fatal(err)
	}
}

func view(t *testing.T, svc *plugins.Service, id string, blocks ...string) error {
	t.Helper()
	raw := make([]json.RawMessage, len(blocks))
	for i, b := range blocks {
		raw[i] = json.RawMessage(b)
	}
	return svc.Message(key, "s1", frame(protocol.TypePluginView, "", protocol.PluginView{PluginID: id, Blocks: raw}))
}

func TestOnlyDevicesWithThePluginsFeatureAreListed(t *testing.T) {
	t.Parallel()
	svc := plugins.NewService(func() {})
	svc.Connect("s1", hello(protocol.FeatureQueries), &fakeDevice{})
	if devs := svc.Devices(); len(devs) != 0 {
		t.Fatalf("Devices = %+v", devs)
	}
	svc.Connect("s2", hello(protocol.FeaturePlugins), &fakeDevice{})
	if devs := svc.Devices(); len(devs) != 1 || devs[0].Key != key || devs[0].SessionID != "s2" {
		t.Fatalf("Devices = %+v", devs)
	}
}

func TestRegisterAndViewKeepRegistrationOrder(t *testing.T) {
	t.Parallel()
	svc, _, notified := connected(t)
	register(t, svc, "zeta", "Zeta")
	register(t, svc, "alpha", "Alpha")
	if err := view(t, svc, "alpha", `{"type":"text","id":"t","text":"hello"}`); err != nil {
		t.Fatal(err)
	}
	register(t, svc, "zeta", "Zeta 2") // re-register keeps the place and the view
	list, err := svc.Plugins(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "zeta" || list[0].Name != "Zeta 2" || list[1].ID != "alpha" {
		t.Fatalf("Plugins = %+v", list)
	}
	if string(list[1].Blocks) != `[{"type":"text","id":"t","text":"hello"}]` || string(list[0].Blocks) != `[]` {
		t.Fatalf("blocks = %s / %s", list[1].Blocks, list[0].Blocks)
	}
	if list[1].UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt not set by a view")
	}
	if notified.Load() < 4 {
		t.Fatalf("notified %d times", notified.Load())
	}
}

func TestInvalidFramesAreRefused(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	if err := svc.Message(key, "s1", frame(protocol.TypePluginRegister, "", protocol.PluginRegister{ID: "Bad Id", Name: "x"})); !errors.Is(err, plugins.ErrInvalidPlugin) {
		t.Fatalf("bad id err = %v", err)
	}
	if err := svc.Message(key, "s1", frame(protocol.TypePluginRegister, "", protocol.PluginRegister{ID: "ok"})); !errors.Is(err, plugins.ErrInvalidPlugin) {
		t.Fatalf("missing name err = %v", err)
	}
	if err := view(t, svc, "nope", `{"type":"text","id":"t","text":"x"}`); !errors.Is(err, plugins.ErrUnknownPlugin) {
		t.Fatalf("view of an unregistered plugin err = %v", err)
	}
	if err := svc.Message(key, "s2", frame(protocol.TypePluginRegister, "", protocol.PluginRegister{ID: "a", Name: "A"})); !errors.Is(err, plugins.ErrStaleSession) {
		t.Fatalf("stale session err = %v", err)
	}
	if list, _ := svc.Plugins(key); len(list) != 0 {
		t.Fatalf("refused frames registered %+v", list)
	}
}

func TestAViewTooLargeShowsAnError(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	register(t, svc, "big", "Big")
	if err := view(t, svc, "big", `{"type":"text","id":"t","text":"ok"}`); err != nil {
		t.Fatal(err)
	}
	huge := `{"type":"text","id":"t","text":"` + strings.Repeat("x", plugins.MaxViewBytes) + `"}`
	if err := view(t, svc, "big", huge); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.Plugins(key)
	if string(list[0].Blocks) != `[]` || !strings.Contains(list[0].Error, "larger than 1 MiB") {
		t.Fatalf("plugin after a huge view = %s, %q", list[0].Blocks, list[0].Error)
	}
	many := make([]string, plugins.MaxBlocks+1)
	for i := range many {
		many[i] = `{"type":"text","id":"t","text":"x"}`
	}
	if err := view(t, svc, "big", many...); err != nil {
		t.Fatal(err)
	}
	if list, _ = svc.Plugins(key); !strings.Contains(list[0].Error, "more than 100 blocks") {
		t.Fatalf("error after too many blocks = %q", list[0].Error)
	}
	if err := view(t, svc, "big", `{"type":"text","id":"t","text":"back"}`); err != nil {
		t.Fatal(err)
	}
	if list, _ = svc.Plugins(key); list[0].Error != "" || !strings.Contains(string(list[0].Blocks), "back") {
		t.Fatalf("a valid view did not clear the error: %+v", list[0])
	}
}

func TestActionRoundTrip(t *testing.T) {
	t.Parallel()
	svc, dev, _ := connected(t)
	register(t, svc, "flags", "Flags")
	dev.answer = func(a protocol.PluginAction) *protocol.PluginResult {
		if a.ActionID == "boom" {
			return &protocol.PluginResult{ID: a.ID, OK: false, Error: "no handler for boom"}
		}
		return &protocol.PluginResult{ID: a.ID, OK: true, Message: "toggled " + a.RowID}
	}
	ctx := context.Background()
	msg, err := svc.Action(ctx, key, "flags", "table", "toggle", "darkMode")
	if err != nil || msg != "toggled darkMode" {
		t.Fatalf("Action = %q, %v", msg, err)
	}
	dev.mu.Lock()
	got := dev.sent[0]
	dev.mu.Unlock()
	if got.PluginID != "flags" || got.BlockID != "table" || got.ActionID != "toggle" || got.RowID != "darkMode" || !strings.HasPrefix(got.ID, "p") {
		t.Fatalf("sent %+v", got)
	}
	if _, err := svc.Action(ctx, key, "flags", "tools", "boom", ""); !errors.Is(err, plugins.ErrActionFailed) || !strings.Contains(err.Error(), "no handler for boom") {
		t.Fatalf("failed action err = %v", err)
	}
	if _, err := svc.Action(ctx, key, "other", "b", "a", ""); !errors.Is(err, plugins.ErrUnknownPlugin) {
		t.Fatalf("unknown plugin err = %v", err)
	}
	if _, err := svc.Action(ctx, "nope|d", "flags", "b", "a", ""); !errors.Is(err, plugins.ErrUnknownDevice) {
		t.Fatalf("unknown device err = %v", err)
	}
	if _, err := svc.Action(ctx, key, "flags", "", "a", ""); !errors.Is(err, plugins.ErrInvalidAction) {
		t.Fatalf("missing block id err = %v", err)
	}
}

func TestActionsEndOnTimeoutAndDisconnect(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t) // the fake never answers
	register(t, svc, "flags", "Flags")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := svc.Action(ctx, key, "flags", "b", "a", ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout err = %v", err)
	}
	errc := make(chan error, 1)
	go func() { _, err := svc.Action(context.Background(), key, "flags", "b", "a", ""); errc <- err }()
	time.Sleep(20 * time.Millisecond)
	svc.Disconnect(key, "s1")
	if err := <-errc; !errors.Is(err, plugins.ErrDisconnected) {
		t.Fatalf("disconnect err = %v", err)
	}
	if devs := svc.Devices(); len(devs) != 0 {
		t.Fatalf("device kept after disconnect: %+v", devs)
	}
	if _, err := svc.Plugins(key); !errors.Is(err, plugins.ErrUnknownDevice) {
		t.Fatalf("Plugins after disconnect err = %v", err)
	}
}

func TestAReconnectReplacesThePlugins(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	register(t, svc, "flags", "Flags")
	svc.Connect("s2", hello(protocol.FeaturePlugins), &fakeDevice{})
	if list, _ := svc.Plugins(key); len(list) != 0 {
		t.Fatalf("plugins of the old session survived: %+v", list)
	}
	svc.Disconnect(key, "s1") // the old session ending later changes nothing
	if devs := svc.Devices(); len(devs) != 1 || devs[0].SessionID != "s2" {
		t.Fatalf("Devices = %+v", devs)
	}
}

func TestRegistrationsAreBounded(t *testing.T) {
	t.Parallel()
	svc, _, _ := connected(t)
	for _, reg := range []protocol.PluginRegister{
		{ID: "long-name", Name: strings.Repeat("n", 65)},
		{ID: "long-desc", Name: "x", Description: strings.Repeat("d", 281)},
		{ID: "long-version", Name: "x", Version: strings.Repeat("1", 33)},
	} {
		if err := svc.Message(key, "s1", frame(protocol.TypePluginRegister, "", reg)); !errors.Is(err, plugins.ErrInvalidPlugin) {
			t.Fatalf("register %s err = %v", reg.ID, err)
		}
	}
	for i := range plugins.MaxPlugins {
		register(t, svc, fmt.Sprintf("p%d", i), "P")
	}
	if err := svc.Message(key, "s1", frame(protocol.TypePluginRegister, "", protocol.PluginRegister{ID: "one-too-many", Name: "X"})); !errors.Is(err, plugins.ErrInvalidPlugin) {
		t.Fatalf("register past the limit err = %v", err)
	}
	register(t, svc, "p0", "Renamed") // re-registering a known plugin still works
	if list, _ := svc.Plugins(key); len(list) != plugins.MaxPlugins || list[0].Name != "Renamed" {
		t.Fatalf("plugins = %d, first %q", len(list), list[0].Name)
	}
}
