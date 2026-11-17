package app_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Reidevl/rn-lens/internal/app"
	"github.com/Reidevl/rn-lens/internal/platform/cdp"
	"github.com/Reidevl/rn-lens/internal/targets"
)

// TestE2E_AttachToRunningApp talks to a real Metro and app. It runs only when
// LENS_E2E_METRO names a Metro base URL (for example http://localhost:8081)
// so the regular suite stays hermetic; the nightly job sets it.
func TestE2E_AttachToRunningApp(t *testing.T) {
	endpoint := os.Getenv("LENS_E2E_METRO")
	if endpoint == "" || testing.Short() {
		t.Skip("set LENS_E2E_METRO=http://localhost:8081 with an app running to enable")
	}

	a := app.New(app.Config{MetroEndpoints: []string{endpoint}, PollInterval: 200 * time.Millisecond})
	sessionCh, cancel := a.Bus().Subscribe(app.TopicSessionChanged, 64)
	defer cancel()
	a.Start()
	t.Cleanup(a.Stop)

	ctx, cancelCtx := context.WithTimeout(context.Background(), 10*time.Second)
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
		t.Fatalf("no reloadable target at %s; targets: %+v", endpoint, a.Targets())
	}
	t.Logf("attaching to %s (%s on %s)", target.Title, target.AppID, target.DeviceName)

	if err := a.Attach(target.ID); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	waitState := func(want targets.State, within time.Duration) targets.Snapshot {
		deadline := time.After(within)
		for {
			snap := a.SessionSnapshot()
			if snap.State == want {
				return snap
			}
			select {
			case <-sessionCh:
			case <-deadline:
				t.Fatalf("session did not reach %s within %s; last %+v", want, within, snap)
			}
		}
	}
	snap := waitState(targets.StateAttached, 10*time.Second)

	deadline := time.After(5 * time.Second)
	for snap.Metadata == nil {
		select {
		case <-sessionCh:
			snap = a.SessionSnapshot()
		case <-deadline:
			t.Fatal("no metadataUpdated within 5 s of attaching")
		}
	}
	t.Logf("metadata: %s %s RN %s on %s", snap.Metadata.AppDisplayName, snap.Metadata.Platform, snap.Metadata.ReactNativeVersion, snap.Metadata.DeviceName)
	if snap.Metadata.ReactNativeVersion == "" || snap.Metadata.Platform == "" {
		t.Fatalf("incomplete metadata: %+v", snap.Metadata)
	}

	// Reload must succeed and the session must survive it (reloadable page).
	if err := a.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	time.Sleep(2 * time.Second)
	if got := a.SessionSnapshot().State; got != targets.StateAttached {
		t.Fatalf("state after reload = %s, want attached", got)
	}

	// The connection still answers requests after the reload.
	if _, err := a.Session().Call(ctx, cdp.MethodRuntimeEnable, nil); err != nil {
		t.Fatalf("Runtime.enable after reload: %v", err)
	}

	// Console: Hermes replays buffered lines on Runtime.enable, so any app
	// yields entries; the example app additionally logs "tick" every two
	// seconds, which is asserted only when it is the attached app.
	isExample := snap.Metadata.AppIdentifier == "host.exp.Exponent" || target.AppID == "host.exp.Exponent"
	consoleDeadline := time.After(6 * time.Second)
	for {
		found := false
		for _, e := range a.Console().Store().Range(1, 0) {
			if e.Source != "console" {
				continue
			}
			if !isExample || (e.Kind == "log" && len(e.Text) >= 4 && e.Text[:4] == "tick") {
				found = true
				break
			}
		}
		if found {
			break
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-consoleDeadline:
			t.Fatalf("no console entry within 6 s of attaching (example app: %v)", isExample)
		}
	}
	result, err := a.Console().Evaluate(ctx, "1+1")
	if err != nil || result.Text != "2" {
		t.Fatalf("Evaluate(1+1) = %+v, %v", result, err)
	}
	t.Logf("console entries: %d; eval 1+1 = %s", a.Console().Store().Stats().Len, result.Text)
}
