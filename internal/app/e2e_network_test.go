package app_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/network"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// TestE2E_NetworkCapturesARequest attaches to a real app on React Native 0.83+
// (LENS_E2E_METRO, as in TestE2E_AttachToRunningApp), sends one request from it
// and checks the request, its status and its body. It never reloads the app
// and starts no SDK server, so it can run next to a Lens window.
func TestE2E_NetworkCapturesARequest(t *testing.T) {
	endpoint := os.Getenv("LENS_E2E_METRO")
	if endpoint == "" || testing.Short() {
		t.Skip("set LENS_E2E_METRO=http://localhost:8081 with an app running to enable")
	}
	const url = "https://httpbin.org/json"

	// Keep the Network frames the app sends so a failure shows what arrived.
	var (
		framesMu sync.Mutex
		frames   []string
	)
	observe := func(dir cdp.Direction, raw []byte) {
		if s := string(raw); strings.Contains(s, `"Network.`) {
			framesMu.Lock()
			frames = append(frames, string(dir)+" "+truncate(s, 600))
			framesMu.Unlock()
		}
	}
	t.Cleanup(func() {
		if t.Failed() {
			framesMu.Lock()
			defer framesMu.Unlock()
			t.Logf("%d Network frames:\n%s", len(frames), strings.Join(frames, "\n"))
		}
	})

	a := app.New(app.Config{MetroEndpoints: []string{endpoint}, PollInterval: 200 * time.Millisecond, SDKAddr: "off", FrameObserver: observe})
	networkCh, cancel := a.Bus().Subscribe(app.TopicNetworkChanged, 256)
	defer cancel()
	a.Start()
	t.Cleanup(a.Stop)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, e := range a.Console().Store().Range(1, 0) {
			if strings.Contains(e.Text, "lens-e2e-xhr") {
				t.Logf("app console: %s", e.Text)
			}
		}
	})

	ctx, cancelCtx := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancelCtx()
	a.RefreshTargets(ctx)
	var target targets.Target
	for _, candidate := range a.Targets() {
		if candidate.Reloadable {
			target = candidate
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("no reloadable target at %s", endpoint)
	}
	if err := a.Attach(target.ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	waitNetwork := func(what string, done func() bool) {
		t.Helper()
		for !done() {
			select {
			case <-networkCh:
			case <-time.After(200 * time.Millisecond):
			case <-ctx.Done():
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	waitNetwork("availability", func() bool { return a.Network().Availability().Checked })
	if avail := a.Network().Availability(); !avail.Supported {
		t.Skipf("network inspection unavailable on this app: %s", avail.Reason)
	}

	if md := a.SessionSnapshot().Metadata; md != nil {
		t.Logf("app: %s on %s, RN %s, networkInspectionEnabled=%v", md.AppDisplayName, md.Platform, md.ReactNativeVersion, md.NetworkInspectionEnabled)
	}
	// XMLHttpRequest goes through React Native's networking module, which
	// reports to the inspector. Expo SDK 58+ replaces the global fetch with
	// expo/fetch, which does not (unless EXPO_PUBLIC_USE_RN_FETCH=1).
	xhr := "(() => { const x = new XMLHttpRequest(); x.onloadend = () => console.log('lens-e2e-xhr', x.status); x.open('GET', '" + url + "'); x.send(); return 'sent'; })()"
	if _, err := a.Console().Evaluate(ctx, xhr); err != nil {
		t.Fatalf("Evaluate XMLHttpRequest: %v", err)
	}
	var row network.Summary
	waitNetwork("a finished "+url+" row", func() bool {
		for _, r := range a.Network().Store().Since(0, 0) {
			if r.URL == url && r.State != network.StatePending {
				row = r
				return true
			}
		}
		return false
	})
	t.Logf("row: %s %s → %d %s, %d bytes, %.0f ms", row.Method, row.URL, row.Status, row.MimeType, row.EncodedSize, (row.EndTime-row.StartTime)*1000)
	if row.State != network.StateComplete || row.Status != 200 || row.Method != http.MethodGet {
		t.Fatalf("row = %+v", row)
	}

	body, err := a.Network().ResponseBody(ctx, row.Seq, row.Epoch)
	if err != nil {
		t.Fatalf("ResponseBody: %v", err)
	}
	if !strings.Contains(body.Text, "slideshow") {
		t.Fatalf("body = %+v, want the httpbin JSON", body)
	}
	har, err := a.Network().ExportHAR(ctx, "e2e")
	if err != nil || !strings.Contains(string(har), url) {
		t.Fatalf("ExportHAR: %v (contains url: %v)", err, strings.Contains(string(har), url))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
