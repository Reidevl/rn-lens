package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/app"
)

// runTargets polls Metro once and prints the targets as JSON. Exit code 2
// means no target was found, which lets CI scripts wait for an app.
func runTargets(args []string) int {
	fs := flag.NewFlagSet("rnlens targets", flag.ContinueOnError)
	endpoints := fs.String("metro", strings.Join(metro.DefaultEndpoints, ","), "comma-separated Metro addresses to poll: URL, host:port or port")
	timeout := fs.Duration("timeout", 3*time.Second, "HTTP timeout per endpoint")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	core := app.New(app.Config{Version: version, MetroEndpoints: metroEndpoints(*endpoints)})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	core.RefreshTargets(ctx)

	list := core.Targets()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(list); err != nil {
		fmt.Fprintf(os.Stderr, "rnlens: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		return 2
	}
	return 0
}

// metroEndpoints parses the --metro flag into Metro base URLs. Each entry may
// be a full URL ("http://localhost:8081"), a host and port
// ("localhost:8081"), or a bare port ("8081", meaning localhost); entries
// without a scheme get http://.
func metroEndpoints(s string) []string {
	entries := splitList(s)
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		switch {
		case strings.Contains(e, "://"):
		case isPort(e):
			e = "http://localhost:" + e
		default:
			e = "http://" + e
		}
		out = append(out, strings.TrimRight(e, "/"))
	}
	return out
}

func isPort(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
