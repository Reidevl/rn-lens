package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeMetro serves GET /json with the given page descriptions.
func fakeMetro(t *testing.T, pages []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(pages)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func fuseboxPage(srvURL string) map[string]any {
	return map[string]any{
		"id":                   "device-1-1",
		"title":                "React Native Bridgeless [C++ connection]",
		"appId":                "com.example.app",
		"webSocketDebuggerUrl": "ws" + strings.TrimPrefix(srvURL, "http") + "/inspector/debug?device=device-1&page=1",
		"deviceName":           "iPhone 17",
		"reactNative": map[string]any{
			"logicalDeviceId": "device-1",
			"capabilities":    map[string]any{"nativePageReloads": true},
		},
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
// Tests using it must not run in parallel.
func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	code := fn()
	_ = w.Close()
	return <-out, code
}

func TestRunPrintsTheVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-v"}, {"version"}} {
		out, code := captureStdout(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("run(%v) = %d, want 0", args, code)
		}
		if want := "rnlens " + buildVersion() + "\n"; out != want {
			t.Fatalf("run(%v) printed %q, want %q", args, out, want)
		}
	}
}

func TestRunPrintsUsageWithExitCodes(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		out, code := captureStdout(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("run(%v) = %d, want 0", args, code)
		}
		for _, want := range []string{"Usage:", "--metro URLS", "rnlens targets", "rnlens record", "Exit codes:", "2  "} {
			if !strings.Contains(out, want) {
				t.Fatalf("run(%v) output lacks %q:\n%s", args, want, out)
			}
		}
	}
}

// -h anywhere in the root flag list prints the same usage as a leading
// --help and exits 0, instead of the FlagSet's generated usage and exit 2.
func TestRunPrintsUsageWhenHelpFollowsOtherFlags(t *testing.T) {
	for _, args := range [][]string{{"--sdk-port", "1", "-h"}, {"--sdk-port", "1", "--help"}, {"--sdk-token", "t", "-h"}} {
		out, code := captureStdout(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("run(%v) = %d, want 0", args, code)
		}
		for _, want := range []string{"Usage:", "rnlens targets", "rnlens record", "Exit codes:"} {
			if !strings.Contains(out, want) {
				t.Fatalf("run(%v) output lacks %q:\n%s", args, want, out)
			}
		}
	}
}

func TestRunExitsTwoOnBadRootFlags(t *testing.T) {
	for _, args := range [][]string{{"--no-such-flag"}, {"--sdk-port", "not-a-number"}} {
		if _, code := captureStdout(t, func() int { return run(args) }); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestTargetsPrintsJSONAndExitsZero(t *testing.T) {
	metro := fakeMetro(t, []map[string]any{fuseboxPage("http://localhost:8081")})

	out, code := captureStdout(t, func() int {
		return run([]string{"targets", "-metro", metro.URL, "-timeout", "2s"})
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var list []struct {
		ID         string `json:"id"`
		Endpoint   string `json:"endpoint"`
		Reloadable bool   `json:"reloadable"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if len(list) != 1 || list[0].ID != metro.URL+"|device-1-1" || list[0].Endpoint != metro.URL || !list[0].Reloadable {
		t.Fatalf("targets = %+v, want one reloadable target from %s", list, metro.URL)
	}
}

func TestTargetsExitsTwoWithoutTargets(t *testing.T) {
	metro := fakeMetro(t, nil)

	out, code := captureStdout(t, func() int {
		return run([]string{"targets", "-metro", metro.URL, "-timeout", "2s"})
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("stdout = %q, want an empty JSON list", out)
	}
}

func TestTargetsExitsTwoOnBadFlags(t *testing.T) {
	if code := run([]string{"targets", "-no-such-flag"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestSplitList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single", "http://localhost:8081", []string{"http://localhost:8081"}},
		{"several", "http://a:1,http://b:2", []string{"http://a:1", "http://b:2"}},
		{"trims spaces and skips empties", " http://a:1 , ,http://b:2,", []string{"http://a:1", "http://b:2"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := splitList(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitList(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRecordRequiresOut(t *testing.T) {
	metro := fakeMetro(t, nil)
	if code := run([]string{"record", "-metro", metro.URL}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRecordDoesNotCreateTheOutputWithoutATarget(t *testing.T) {
	metro := fakeMetro(t, nil)
	out := filepath.Join(t.TempDir(), "capture.jsonl")

	if code := run([]string{"record", "-metro", metro.URL, "-out", out}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%s) = %v, want the file not to exist when no target was found", out, err)
	}
}
