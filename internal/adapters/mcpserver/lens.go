// Package mcpserver exposes the running Lens to AI clients over the Model
// Context Protocol: the app's console, network, queries, mutations, storage
// and plugin panels, to read and to change. It talks to Lens through the
// Lens interface so its tools are tested without a real app.
package mcpserver

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/console"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/perf"
	"github.com/Reidevl/rn-lens/internal/plugins"
	"github.com/Reidevl/rn-lens/internal/protocol"
	"github.com/Reidevl/rn-lens/internal/queries"
	"github.com/Reidevl/rn-lens/internal/storage"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// Lens is what the tools read and change.
type Lens interface {
	Info() app.Info
	Session() targets.Snapshot
	Targets() []targets.Target
	Attach(id string) error
	Reload(ctx context.Context) error

	// ConsoleTail returns the newest entries, oldest first.
	ConsoleTail(limit int) []console.Entry
	Evaluate(ctx context.Context, expression string) (console.Entry, error)

	// NetworkRequests returns every captured request, newest first.
	NetworkRequests() []network.Summary
	NetworkRequest(seq uint64) (network.Request, bool)
	ResponseBody(ctx context.Context, seq, epoch uint64) (network.Body, error)
	RequestBody(ctx context.Context, seq, epoch uint64) (string, error)

	SDKDevices() []queries.Device
	Queries(device string) ([]queries.Query, error)
	Query(device, hash string) (queries.Query, error)
	Mutations(device string) ([]protocol.Mutation, error)
	QueryAction(ctx context.Context, device, action, hash string, data json.RawMessage) error
	SetOnline(device string, online bool) error

	StorageDevices() []storage.Device
	Stores(device string) ([]protocol.StorageStore, error)
	StorageGet(ctx context.Context, device, store, key string) (protocol.StorageValue, error)
	StorageSet(ctx context.Context, device, store, key string, v protocol.StorageValue) error
	StorageRemove(ctx context.Context, device, store, key string) error
	StorageClear(ctx context.Context, device, store string) error

	PluginDevices() []plugins.Device
	Plugins(device string) ([]plugins.Plugin, error)
	PluginAction(ctx context.Context, device, plugin, block, action, row string) (string, error)

	Screenshot(ctx context.Context, format string) (perf.Image, error)
}

// AppLens adapts the running app to Lens.
type AppLens struct{ App *app.App }

var _ Lens = AppLens{}

func (l AppLens) Info() app.Info                   { return l.App.Info() }
func (l AppLens) Session() targets.Snapshot        { return l.App.SessionSnapshot() }
func (l AppLens) Targets() []targets.Target        { return l.App.Targets() }
func (l AppLens) Attach(id string) error           { return l.App.Attach(id) }
func (l AppLens) Reload(ctx context.Context) error { return l.App.Reload(ctx) }
func (l AppLens) SDKDevices() []queries.Device     { return l.App.Queries().Devices() }
func (l AppLens) StorageDevices() []storage.Device { return l.App.Storage().Devices() }
func (l AppLens) PluginDevices() []plugins.Device  { return l.App.Plugins().Devices() }
func (l AppLens) Stores(d string) ([]protocol.StorageStore, error) {
	return l.App.Storage().Stores(d)
}

func (l AppLens) ConsoleTail(limit int) []console.Entry {
	st := l.App.Console().Store()
	stats := st.Stats()
	from := stats.OldestSeq
	if limit > 0 {
		n := uint64(limit)
		if stats.LastSeq >= n && stats.LastSeq-n+1 > from {
			from = stats.LastSeq - n + 1
		}
	}
	return st.Range(from, limit)
}

func (l AppLens) Evaluate(ctx context.Context, expression string) (console.Entry, error) {
	return l.App.Console().Evaluate(ctx, expression)
}

func (l AppLens) NetworkRequests() []network.Summary {
	rows := l.App.Network().Store().Since(0, 0)
	slices.SortFunc(rows, func(a, b network.Summary) int { return cmp.Compare(b.Seq, a.Seq) })
	return rows
}

func (l AppLens) NetworkRequest(seq uint64) (network.Request, bool) {
	return l.App.Network().Store().Get(seq)
}

func (l AppLens) ResponseBody(ctx context.Context, seq, epoch uint64) (network.Body, error) {
	return l.App.Network().ResponseBody(ctx, seq, epoch)
}

func (l AppLens) RequestBody(ctx context.Context, seq, epoch uint64) (string, error) {
	return l.App.Network().RequestBody(ctx, seq, epoch)
}

func (l AppLens) Queries(d string) ([]queries.Query, error) {
	list, _, err := l.App.Queries().Queries(d)
	return list, err
}

func (l AppLens) Query(d, hash string) (queries.Query, error) { return l.App.Queries().Query(d, hash) }

func (l AppLens) Mutations(d string) ([]protocol.Mutation, error) {
	return l.App.Queries().Mutations(d)
}

func (l AppLens) QueryAction(ctx context.Context, d, action, hash string, data json.RawMessage) error {
	return l.App.Queries().Action(ctx, d, action, hash, data)
}

func (l AppLens) SetOnline(d string, online bool) error { return l.App.Queries().SetOnline(d, online) }

func (l AppLens) StorageGet(ctx context.Context, d, store, key string) (protocol.StorageValue, error) {
	return l.App.Storage().Get(ctx, d, store, key)
}

func (l AppLens) StorageSet(ctx context.Context, d, store, key string, v protocol.StorageValue) error {
	return l.App.Storage().Set(ctx, d, store, key, v)
}

func (l AppLens) StorageRemove(ctx context.Context, d, store, key string) error {
	return l.App.Storage().Remove(ctx, d, store, key)
}

func (l AppLens) StorageClear(ctx context.Context, d, store string) error {
	return l.App.Storage().Clear(ctx, d, store)
}

func (l AppLens) Plugins(d string) ([]plugins.Plugin, error) { return l.App.Plugins().Plugins(d) }

func (l AppLens) PluginAction(ctx context.Context, d, plugin, block, action, row string) (string, error) {
	return l.App.Plugins().Action(ctx, d, plugin, block, action, row)
}

func (l AppLens) Screenshot(ctx context.Context, format string) (perf.Image, error) {
	return l.App.Performance().Screenshot(ctx, format, 80)
}
