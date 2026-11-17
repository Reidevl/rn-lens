package metro_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/metro"
)

// fakeMetro serves /json from a mutable page list and records /open-debugger calls.
type fakeMetro struct {
	mu      sync.Mutex
	pages   []metro.Page
	failing bool
	opened  []string
	srv     *httptest.Server
}

func newFakeMetro(t *testing.T) *fakeMetro {
	t.Helper()
	f := &fakeMetro{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /json", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failing {
			http.Error(w, "bundling", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(f.pages)
	})
	mux.HandleFunc("GET /json/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"Browser": "Mobile JavaScript", "Protocol-Version": "1.1"})
	})
	mux.HandleFunc("POST /open-debugger", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		target := r.URL.Query().Get("target")
		f.opened = append(f.opened, target)
		if target == "missing" {
			http.Error(w, "Unable to find debugger target", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeMetro) setPages(pages ...metro.Page) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = pages
}

// setFailing makes GET /json answer 503 until cleared, standing in for a Metro
// whose event loop is busy bundling and misses the client timeout.
func (f *fakeMetro) setFailing(failing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = failing
}

func fuseboxPage(id, title string) metro.Page {
	return metro.Page{
		ID:                   "device-1-" + id,
		Title:                title,
		AppID:                "com.example.app",
		Description:          "com.example.app",
		Type:                 "node",
		WebSocketDebuggerURL: "ws://localhost:8081/inspector/debug?device=device-1&page=" + id,
		DeviceName:           "iPhone 17",
		ReactNative: metro.ReactNativeInfo{
			LogicalDeviceID: "device-1",
			Capabilities: metro.Capabilities{
				NativePageReloads:         true,
				NativeSourceCodeFetching:  true,
				SupportsMultipleDebuggers: true,
			},
		},
	}
}

func TestListPagesParsesPageDescriptions(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	f.setPages(fuseboxPage("1", "React Native"))
	client := metro.NewClient(nil)

	pages, err := client.ListPages(context.Background(), f.srv.URL)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	p := pages[0]
	if p.PageID() != "1" {
		t.Fatalf("PageID = %q, want 1", p.PageID())
	}
	if !p.IsReloadable() {
		t.Fatal("IsReloadable = false, want true for nativePageReloads")
	}
	if !p.ReactNative.Capabilities.SupportsMultipleDebuggers {
		t.Fatal("capabilities not parsed")
	}
}

func TestIsReloadableFallsBackToLegacyTitle(t *testing.T) {
	t.Parallel()
	legacy := metro.Page{ID: "x", Title: "React Native Experimental (Improved Chrome Reloads)"}
	if !legacy.IsReloadable() {
		t.Fatal("legacy improved-reloads title should count as reloadable")
	}
	plain := metro.Page{ID: "y", Title: "Hermes React Native"}
	if plain.IsReloadable() {
		t.Fatal("plain page should not be reloadable")
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	v, err := metro.NewClient(nil).Version(context.Background(), f.srv.URL)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.ProtocolVersion != "1.1" || v.Browser != "Mobile JavaScript" {
		t.Fatalf("got %+v", v)
	}
}

func TestOpenDebugger(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	client := metro.NewClient(nil)

	if err := client.OpenDebugger(context.Background(), f.srv.URL, "device-1-1"); err != nil {
		t.Fatalf("OpenDebugger: %v", err)
	}
	if err := client.OpenDebugger(context.Background(), f.srv.URL, "missing"); err == nil {
		t.Fatal("OpenDebugger for a missing target succeeded, want error")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.opened) != 2 || f.opened[0] != "device-1-1" {
		t.Fatalf("opened = %v", f.opened)
	}
}

func TestListPagesWhenMetroIsDown(t *testing.T) {
	t.Parallel()
	client := metro.NewClient(&http.Client{Timeout: 200 * time.Millisecond})
	if _, err := client.ListPages(context.Background(), "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected an error for a closed port")
	}
}

type changeRecorder struct {
	mu      sync.Mutex
	changes []metro.Change
	notify  chan struct{}
}

func newChangeRecorder() *changeRecorder {
	return &changeRecorder{notify: make(chan struct{}, 64)}
}

func (r *changeRecorder) onChange(changes []metro.Change) {
	r.mu.Lock()
	r.changes = append(r.changes, changes...)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *changeRecorder) wait(t *testing.T, want int) []metro.Change {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		r.mu.Lock()
		if len(r.changes) >= want {
			out := append([]metro.Change(nil), r.changes...)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("timed out waiting for %d changes", want)
		}
	}
}

func TestDiscoveryEmitsAddedUpdatedRemoved(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	f.setPages(fuseboxPage("1", "React Native"))
	rec := newChangeRecorder()
	d := metro.NewDiscovery(metro.NewClient(nil), metro.DiscoveryOptions{
		Endpoints: []string{f.srv.URL},
		Interval:  10 * time.Millisecond,
		OnChange:  rec.onChange,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	changes := rec.wait(t, 1)
	if changes[0].Kind != metro.Added || changes[0].Item.Page.PageID() != "1" {
		t.Fatalf("first change = %+v, want added page 1", changes[0])
	}
	if got := d.Snapshot(); len(got) != 1 || got[0].Endpoint != f.srv.URL {
		t.Fatalf("Snapshot = %+v", got)
	}
	if !d.Reachable(f.srv.URL) {
		t.Fatal("endpoint should be reachable")
	}
	gen := d.Generation()

	renamed := fuseboxPage("1", "React Native (renamed)")
	f.setPages(renamed)
	changes = rec.wait(t, 2)
	if changes[1].Kind != metro.Updated || changes[1].Item.Page.Title != "React Native (renamed)" {
		t.Fatalf("second change = %+v, want updated title", changes[1])
	}
	if d.Generation() <= gen {
		t.Fatal("Generation did not advance on update")
	}

	f.setPages()
	changes = rec.wait(t, 3)
	if changes[2].Kind != metro.Removed {
		t.Fatalf("third change = %+v, want removed", changes[2])
	}
	if got := d.Snapshot(); len(got) != 0 {
		t.Fatalf("Snapshot after removal = %+v, want empty", got)
	}
}

func TestDiscoverySurvivesUnreachableEndpoint(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	f.setPages(fuseboxPage("1", "React Native"))
	rec := newChangeRecorder()
	d := metro.NewDiscovery(metro.NewClient(&http.Client{Timeout: 200 * time.Millisecond}), metro.DiscoveryOptions{
		Endpoints:  []string{"http://127.0.0.1:1", f.srv.URL},
		Interval:   10 * time.Millisecond,
		MaxBackoff: 20 * time.Millisecond,
		OnChange:   rec.onChange,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Run(ctx)

	rec.wait(t, 1)
	if d.Reachable("http://127.0.0.1:1") {
		t.Fatal("closed port reported reachable")
	}
	if got := d.Snapshot(); len(got) != 1 {
		t.Fatalf("Snapshot = %+v, want the one reachable page", got)
	}
}

func TestDiscoveryRefreshIsSynchronous(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	f.setPages(fuseboxPage("1", "React Native"), fuseboxPage("2", "Other"))
	d := metro.NewDiscovery(metro.NewClient(nil), metro.DiscoveryOptions{Endpoints: []string{f.srv.URL}})

	d.Refresh(context.Background())

	if got := d.Snapshot(); len(got) != 2 || got[0].Page.PageID() != "1" || got[1].Page.PageID() != "2" {
		t.Fatalf("Snapshot = %+v, want pages 1 and 2 sorted", got)
	}
}

// syncRecorder collects changes emitted by the synchronous Refresh path.
type syncRecorder struct {
	changes []metro.Change
}

func (r *syncRecorder) onChange(changes []metro.Change) {
	r.changes = append(r.changes, changes...)
}

func (r *syncRecorder) count(kind metro.ChangeKind) int {
	n := 0
	for _, c := range r.changes {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

// discoverOne returns a discovery whose first Refresh has already listed page 1
// of f, so each test starts from an attached target.
func discoverOne(t *testing.T, f *fakeMetro, rec *syncRecorder, failuresBeforeRemoval int) *metro.Discovery {
	t.Helper()
	f.setPages(fuseboxPage("1", "React Native"))
	d := metro.NewDiscovery(metro.NewClient(nil), metro.DiscoveryOptions{
		Endpoints:             []string{f.srv.URL},
		FailuresBeforeRemoval: failuresBeforeRemoval,
		OnChange:              rec.onChange,
	})
	d.Refresh(context.Background())
	if got := d.Snapshot(); len(got) != 1 {
		t.Fatalf("Snapshot after first poll = %+v, want one page", got)
	}
	return d
}

func TestDiscoveryKeepsPagesUntilConsecutiveFailuresReachThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		threshold   int
		failures    int
		wantRemoved bool
	}{
		{name: "default keeps pages after two failures", threshold: 0, failures: 2, wantRemoved: false},
		{name: "default removes pages on the third failure", threshold: 0, failures: 3, wantRemoved: true},
		{name: "threshold one removes on the first failure", threshold: 1, failures: 1, wantRemoved: true},
		{name: "threshold five keeps pages after four failures", threshold: 5, failures: 4, wantRemoved: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeMetro(t)
			rec := &syncRecorder{}
			d := discoverOne(t, f, rec, tt.threshold)

			f.setFailing(true)
			for i := 0; i < tt.failures; i++ {
				d.Refresh(context.Background())
				if d.Reachable(f.srv.URL) {
					t.Fatalf("Reachable = true after failure %d, want false", i+1)
				}
			}

			wantRemoved, wantPages := 0, 1
			if tt.wantRemoved {
				wantRemoved, wantPages = 1, 0
			}
			if got := rec.count(metro.Removed); got != wantRemoved {
				t.Fatalf("removed events = %d, want %d (changes %+v)", got, wantRemoved, rec.changes)
			}
			if got := d.Snapshot(); len(got) != wantPages {
				t.Fatalf("Snapshot = %+v, want %d pages", got, wantPages)
			}
		})
	}
}

func TestDiscoverySuccessfulPollResetsFailureCount(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	rec := &syncRecorder{}
	d := discoverOne(t, f, rec, 0)

	// Two failures, one success, two more failures: never three in a row.
	f.setFailing(true)
	d.Refresh(context.Background())
	d.Refresh(context.Background())
	f.setFailing(false)
	d.Refresh(context.Background())
	if !d.Reachable(f.srv.URL) {
		t.Fatal("Reachable = false after a successful poll, want true")
	}
	f.setFailing(true)
	d.Refresh(context.Background())
	d.Refresh(context.Background())

	if got := rec.count(metro.Removed); got != 0 {
		t.Fatalf("removed events = %d, want 0 (changes %+v)", got, rec.changes)
	}
	if got := d.Snapshot(); len(got) != 1 {
		t.Fatalf("Snapshot = %+v, want the page kept", got)
	}
	if got := rec.count(metro.Added); got != 1 {
		t.Fatalf("added events = %d, want the single initial add", got)
	}
}

func TestDiscoveryHealthyEndpointStillAddsUpdatesAndRemoves(t *testing.T) {
	t.Parallel()
	f := newFakeMetro(t)
	rec := &syncRecorder{}
	d := discoverOne(t, f, rec, 0)

	f.setPages(fuseboxPage("1", "React Native (renamed)"))
	d.Refresh(context.Background())
	// Metro itself drops the page: removal is immediate, the failure threshold
	// only applies to polls that fail.
	f.setPages()
	d.Refresh(context.Background())

	kinds := make([]metro.ChangeKind, 0, len(rec.changes))
	for _, c := range rec.changes {
		kinds = append(kinds, c.Kind)
	}
	want := []metro.ChangeKind{metro.Added, metro.Updated, metro.Removed}
	if len(kinds) != len(want) {
		t.Fatalf("changes = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("changes = %v, want %v", kinds, want)
		}
	}
	if got := d.Snapshot(); len(got) != 0 {
		t.Fatalf("Snapshot = %+v, want empty", got)
	}
	if !d.Reachable(f.srv.URL) {
		t.Fatal("Reachable = false for a healthy endpoint, want true")
	}
}
