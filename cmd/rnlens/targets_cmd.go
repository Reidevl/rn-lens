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
	endpoints := fs.String("metro", strings.Join(metro.DefaultEndpoints, ","), "comma-separated Metro base URLs to poll")
	timeout := fs.Duration("timeout", 3*time.Second, "HTTP timeout per endpoint")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	core := app.New(app.Config{Version: version, MetroEndpoints: splitList(*endpoints)})
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

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
