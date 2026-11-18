package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/perf"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/plugins"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
	"github.com/Reidevl/rn-lens/internal/storage"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// fakeLens records the changes tools ask for and serves canned data.
type fakeLens struct {
	mu        sync.Mutex
	calls     []string
	devices   []queries.Device
	console   []console.Entry
	reqs      []network.Summary
	queries   []queries.Query
	stores    []protocol.StorageStore
	set       []protocol.StorageValue
	evalErr   bool
	session   targets.Snapshot
	mutations []protocol.Mutation
}

// attachedTo is a session attached to an app on the named device.
func attachedTo(deviceName, platform, rn string) targets.Snapshot {
	return targets.Snapshot{State: targets.StateAttached, Metadata: &targets.Metadata{DeviceName: deviceName, Platform: platform, ReactNativeVersion: rn}}
}

func (f *fakeLens) record(c string) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}

func (f *fakeLens) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func device(appID, id string, features ...string) queries.Device {
	return queries.Device{Key: appID + "|" + id, App: protocol.AppInfo{ID: appID}, Device: protocol.DeviceInfo{ID: id}, Features: features}
}

func (f *fakeLens) Info() app.Info { return app.Info{Name: "Lens", Version: "9.9.9"} }
func (f *fakeLens) Session() targets.Snapshot {
	if f.session.State == "" {
		return targets.Snapshot{State: targets.StateAttached}
	}
	return f.session
}

func (f *fakeLens) Targets() []targets.Target { return []targets.Target{{ID: "t1", Title: "Example"}} }
func (f *fakeLens) Attach(id string) error    { f.record("attach:" + id); return nil }
func (f *fakeLens) Reload(context.Context) error {
	f.record("reload")
	return nil
}

func (f *fakeLens) ConsoleTail(limit int) []console.Entry {
	if limit < len(f.console) {
		return f.console[len(f.console)-limit:]
	}
	return f.console
}

func (f *fakeLens) Evaluate(_ context.Context, expr string) (console.Entry, error) {
	f.record("eval:" + expr)
	if f.evalErr {
		return console.Entry{Kind: console.KindException, Text: "ReferenceError: nope is not defined"}, nil
	}
	return console.Entry{Kind: console.KindResult, Text: "2", Args: []cdp.RemoteObject{{Type: "number", Value: json.RawMessage("2")}}}, nil
}
func (f *fakeLens) NetworkRequests() []network.Summary { return f.reqs }
func (f *fakeLens) NetworkRequest(seq uint64) (network.Request, bool) {
	for _, r := range f.reqs {
		if r.Seq == seq {
			return network.Request{Summary: r}, true
		}
	}
	return network.Request{}, false
}

func (f *fakeLens) ResponseBody(context.Context, uint64, uint64) (network.Body, error) {
	return network.Body{Text: `{"ok":true}`, Size: 11}, nil
}

func (f *fakeLens) RequestBody(context.Context, uint64, uint64) (string, error) {
	return `{"q":1}`, nil
}
func (f *fakeLens) SDKDevices() []queries.Device { return f.devices }
func (f *fakeLens) Queries(d string) ([]queries.Query, error) {
	f.record("queries:" + d)
	return f.queries, nil
}

func (f *fakeLens) Query(d, hash string) (queries.Query, error) {
	for _, q := range f.queries {
		if q.QueryHash == hash {
			return q, nil
		}
	}
	return queries.Query{}, errors.New("queries: unknown query")
}
func (f *fakeLens) Mutations(string) ([]protocol.Mutation, error) { return f.mutations, nil }
func (f *fakeLens) QueryAction(_ context.Context, d, action, hash string, data json.RawMessage) error {
	f.record("action:" + d + ":" + action + ":" + hash + ":" + string(data))
	return nil
}

func (f *fakeLens) SetOnline(d string, online bool) error {
	f.record("online:" + d + ":" + map[bool]string{true: "on", false: "off"}[online])
	return nil
}

func (f *fakeLens) StorageDevices() []storage.Device {
	out := make([]storage.Device, 0, len(f.devices))
	for _, d := range f.devices {
		out = append(out, storage.Device{Key: d.Key})
	}
	return out
}
func (f *fakeLens) Stores(string) ([]protocol.StorageStore, error) { return f.stores, nil }
func (f *fakeLens) StorageGet(_ context.Context, _, store, key string) (protocol.StorageValue, error) {
	return protocol.StorageValue{Type: "string", Value: store + "/" + key}, nil
}

func (f *fakeLens) StorageSet(_ context.Context, d, store, key string, v protocol.StorageValue) error {
	f.record("set:" + d + ":" + store + ":" + key)
	f.mu.Lock()
	f.set = append(f.set, v)
	f.mu.Unlock()
	return nil
}

func (f *fakeLens) StorageRemove(_ context.Context, d, store, key string) error {
	f.record("remove:" + d + ":" + store + ":" + key)
	return nil
}

func (f *fakeLens) StorageClear(_ context.Context, d, store string) error {
	f.record("clear:" + d + ":" + store)
	return nil
}

func (f *fakeLens) PluginDevices() []plugins.Device {
	out := make([]plugins.Device, 0, len(f.devices))
	for _, d := range f.devices {
		out = append(out, plugins.Device{Key: d.Key})
	}
	return out
}

func (f *fakeLens) Plugins(string) ([]plugins.Plugin, error) {
	return []plugins.Plugin{{ID: "feature-flags", Name: "Feature flags", Blocks: json.RawMessage(`[]`)}}, nil
}

func (f *fakeLens) PluginAction(_ context.Context, d, plugin, block, action, row string) (string, error) {
	f.record("plugin:" + d + ":" + plugin + ":" + block + ":" + action + ":" + row)
	return row + " is on", nil
}

func (f *fakeLens) Screenshot(_ context.Context, format string) (perf.Image, error) {
	return perf.Image{Format: format, Data: []byte("\x89PNG")}, nil
}

var _ Lens = (*fakeLens)(nil)
