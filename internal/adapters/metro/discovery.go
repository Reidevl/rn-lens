package metro

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// DefaultEndpoints are the Metro base URLs Lens polls when none are configured:
// the React Native default, a common second instance, and the legacy Expo port.
var DefaultEndpoints = []string{
	"http://localhost:8081",
	"http://localhost:8082",
	"http://localhost:19000",
}

// DefaultFailuresBeforeRemoval is how many consecutive failed polls an endpoint
// tolerates before its pages are removed. Metro's event loop stalls while it
// bundles, so a single missed /json must not drop an attached target.
const DefaultFailuresBeforeRemoval = 3

// Discovered is a page together with the Metro endpoint that listed it.
type Discovered struct {
	Endpoint string
	Page     Page
}

// Key identifies a discovered page across polls.
func (d Discovered) Key() string { return d.Endpoint + "|" + d.Page.ID }

// ChangeKind classifies a diff entry.
type ChangeKind string

// Change kinds.
const (
	Added   ChangeKind = "added"
	Updated ChangeKind = "updated"
	Removed ChangeKind = "removed"
)

// Change is one diff entry between two polls.
type Change struct {
	Kind ChangeKind
	Item Discovered
}

// DiscoveryOptions configures a Discovery.
type DiscoveryOptions struct {
	// Endpoints are Metro base URLs. Empty means DefaultEndpoints.
	Endpoints []string
	// Interval between polls of a healthy endpoint. Zero means one second.
	Interval time.Duration
	// MaxBackoff caps the poll interval of an unreachable endpoint. Zero means five seconds.
	MaxBackoff time.Duration
	// FailuresBeforeRemoval is how many consecutive failed polls an endpoint
	// tolerates before its pages are removed. Zero means DefaultFailuresBeforeRemoval.
	FailuresBeforeRemoval int
	// OnChange receives every non-empty diff. It is called from the polling
	// goroutine and must not block.
	OnChange func(changes []Change)
	// Logger receives reachability transitions. Nil means slog.Default().
	Logger *slog.Logger
}

// Discovery polls Metro endpoints and keeps the current set of pages.
type Discovery struct {
	client *Client
	opts   DiscoveryOptions

	mu         sync.RWMutex
	pages      map[string]Discovered
	reachable  map[string]bool
	failures   map[string]int
	generation uint64
}

// NewDiscovery creates a discovery that uses client for requests.
func NewDiscovery(client *Client, opts DiscoveryOptions) *Discovery {
	if len(opts.Endpoints) == 0 {
		opts.Endpoints = DefaultEndpoints
	}
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 5 * time.Second
	}
	if opts.FailuresBeforeRemoval <= 0 {
		opts.FailuresBeforeRemoval = DefaultFailuresBeforeRemoval
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Discovery{
		client:    client,
		opts:      opts,
		pages:     make(map[string]Discovered),
		reachable: make(map[string]bool),
		failures:  make(map[string]int),
	}
}

// Run polls every endpoint until ctx is cancelled. Each endpoint has its own
// goroutine so a slow or dead Metro never delays the others.
func (d *Discovery) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, endpoint := range d.opts.Endpoints {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.pollLoop(ctx, endpoint)
		}()
	}
	wg.Wait()
}

// Snapshot returns the current pages sorted by endpoint then page id.
func (d *Discovery) Snapshot() []Discovered {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Discovered, 0, len(d.pages))
	for _, p := range d.pages {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Endpoint != out[j].Endpoint {
			return out[i].Endpoint < out[j].Endpoint
		}
		return out[i].Page.ID < out[j].Page.ID
	})
	return out
}

// Generation increases every time the snapshot changes, so callers can cheaply
// detect staleness.
func (d *Discovery) Generation() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.generation
}

// Reachable reports whether the last poll of endpoint succeeded.
func (d *Discovery) Reachable(endpoint string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.reachable[endpoint]
}

// Refresh polls every endpoint once, synchronously. Useful for CLI use and tests.
func (d *Discovery) Refresh(ctx context.Context) {
	for _, endpoint := range d.opts.Endpoints {
		d.pollOnce(ctx, endpoint)
	}
}

func (d *Discovery) pollLoop(ctx context.Context, endpoint string) {
	wait := d.opts.Interval
	for {
		ok := d.pollOnce(ctx, endpoint)
		if ok {
			wait = d.opts.Interval
		} else {
			wait = min(wait*2, d.opts.MaxBackoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// pollOnce fetches one endpoint and applies the diff. It reports reachability.
// A failed poll marks the endpoint unreachable at once but keeps its pages
// until FailuresBeforeRemoval consecutive polls have failed, so a Metro that
// is busy bundling does not make the targets list flap.
func (d *Discovery) pollOnce(ctx context.Context, endpoint string) bool {
	pages, err := d.client.ListPages(ctx, endpoint)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		d.setReachable(endpoint, false)
		if d.recordFailure(endpoint) >= d.opts.FailuresBeforeRemoval {
			d.apply(endpoint, nil)
		}
		return false
	}
	d.setReachable(endpoint, true)
	d.resetFailures(endpoint)
	d.apply(endpoint, pages)
	return true
}

// recordFailure counts one more consecutive failed poll of endpoint and
// returns the new count.
func (d *Discovery) recordFailure(endpoint string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failures[endpoint]++
	return d.failures[endpoint]
}

func (d *Discovery) resetFailures(endpoint string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.failures, endpoint)
}

func (d *Discovery) setReachable(endpoint string, ok bool) {
	d.mu.Lock()
	was, known := d.reachable[endpoint]
	d.reachable[endpoint] = ok
	d.mu.Unlock()
	if known && was == ok {
		return
	}
	if ok {
		d.opts.Logger.Info("metro reachable", "endpoint", endpoint)
	} else if known {
		d.opts.Logger.Info("metro unreachable", "endpoint", endpoint)
	}
}

// apply replaces the pages of endpoint with pages and emits the diff.
func (d *Discovery) apply(endpoint string, pages []Page) {
	var changes []Change

	d.mu.Lock()
	seen := make(map[string]struct{}, len(pages))
	for _, p := range pages {
		item := Discovered{Endpoint: endpoint, Page: p}
		key := item.Key()
		seen[key] = struct{}{}
		prev, exists := d.pages[key]
		switch {
		case !exists:
			changes = append(changes, Change{Kind: Added, Item: item})
		case prev.Page != p:
			changes = append(changes, Change{Kind: Updated, Item: item})
		default:
			continue
		}
		d.pages[key] = item
	}
	for key, prev := range d.pages {
		if prev.Endpoint != endpoint {
			continue
		}
		if _, ok := seen[key]; !ok {
			delete(d.pages, key)
			changes = append(changes, Change{Kind: Removed, Item: prev})
		}
	}
	if len(changes) > 0 {
		d.generation++
	}
	d.mu.Unlock()

	if len(changes) > 0 && d.opts.OnChange != nil {
		d.opts.OnChange(changes)
	}
}
