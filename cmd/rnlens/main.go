// Command rnlens starts Lens, React Native DevTools: a desktop window, or in
// the server build (-tags server) a local web server opened in the browser.
//
// Usage:
//
//	rnlens [--sdk-port N] [--sdk-token T]   open Lens
//	rnlens targets         print discovered targets as JSON and exit
//	rnlens record ...      record the CDP traffic of a target to a JSONL file
//	rnlens --version       print the build version
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Reidevl/rn-lens/build"
	"github.com/Reidevl/rn-lens/frontend"
	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
	"github.com/Reidevl/rn-lens/internal/adapters/metro"
	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
	"github.com/Reidevl/rn-lens/internal/app"
)

// version can be overridden at build time with -ldflags "-X main.version=<semver>".
// When it is left empty, the version embedded from build/config.yml is used.
var version = ""

// buildVersion returns the version this binary reports.
func buildVersion() string {
	if version != "" {
		return version
	}
	return build.Version()
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			fmt.Println("rnlens " + buildVersion())
			return 0
		case "targets":
			return runTargets(args[1:])
		case "record":
			return runRecord(args[1:])
		case "--help", "-h", "help":
			printUsage()
			return 0
		}
	}

	fs := flag.NewFlagSet("rnlens", flag.ContinueOnError)
	// -h anywhere in the flag list gets the same usage as a leading --help;
	// the FlagSet's generated one would not mention the subcommands.
	fs.Usage = func() {}
	sdkPort := fs.Int("sdk-port", 7420, "port the @rnlens/sdk WebSocket server listens on (0 disables it)")
	sdkToken := fs.String("sdk-token", "", "pairing token the SDK must present (optional)")
	endpoints := fs.String("metro", strings.Join(metro.DefaultEndpoints, ","), "comma-separated Metro base URLs to poll")
	mcpPort := fs.Int("mcp-port", mcpserver.DefaultPort, "port of the MCP endpoint for AI clients (0 disables it)")
	mcpTokenFlag := fs.String("mcp-token", "", "bearer token AI clients must send to the MCP endpoint (prefer "+mcpTokenEnv+")")
	mcpReadOnly := fs.Bool("mcp-read-only", false, "refuse the MCP tools that change the app (switch it in the MCP panel)")
	mode := registerModeFlags(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage()
			return 0
		}
		// The FlagSet already reported the error on stderr.
		fmt.Fprintln(os.Stderr, `Run "rnlens -h" for usage.`)
		return 2
	}
	sdkAddr := fmt.Sprintf(":%d", *sdkPort)
	if *sdkPort == 0 {
		sdkAddr = "off"
	}

	// Before anything listens: a busy UI port must not leave the SDK server
	// of this process running against another Lens.
	if err := mode.start(); err != nil {
		fmt.Fprintf(os.Stderr, "rnlens: %v\n", err)
		return 1
	}
	core := app.New(app.Config{Version: buildVersion(), SDKAddr: sdkAddr, SDKToken: *sdkToken, MetroEndpoints: splitList(*endpoints)})
	core.Start()
	defer core.Stop()

	opts := mode.options()
	token, warning := mcpToken(*mcpTokenFlag, os.Getenv(mcpTokenEnv))
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	ep, msg := startMCP(core, *mcpPort, token, *mcpReadOnly)
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	if ep != nil {
		defer func() { _ = ep.Close() }()
		opts.MCP = wailsui.MCPInfo{URL: ep.URL, Token: token}
		opts.MCPControl = ep.Control
		fmt.Fprintf(os.Stderr, "MCP for AI clients at %s\n", ep.URL)
	}
	if err := wailsui.Run(core, frontend.Dist, opts); err != nil {
		fmt.Fprintf(os.Stderr, "rnlens: %v\n", err)
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Print(`Lens - React Native DevTools

Usage:
` + runUsage + `    --metro URLS         Comma-separated Metro base URLs to poll (default
                         localhost:8081, 8082 and 19000)
    --sdk-port N         Port for the @rnlens/sdk server (default 7420; 0 disables it)
    --sdk-token T        Pairing token the SDK must present (optional)
    --mcp-port N         Port of the MCP endpoint for AI clients (default 7431;
                         0 disables it)
    --mcp-token T        Bearer token AI clients must send (optional; prefer
                         the RNLENS_MCP_TOKEN environment variable, which
                         other users cannot read)
    --mcp-read-only      Refuse the MCP tools that change the app
  rnlens targets         Print discovered targets as JSON and exit
  rnlens record          Record a target's CDP traffic to a JSONL file
  rnlens --version       Print the build version

Exit codes:
  0  Success
  1  Error
  2  Bad usage, or no target found (targets, record)

Run "rnlens <command> -h" for command flags.
`)
}
