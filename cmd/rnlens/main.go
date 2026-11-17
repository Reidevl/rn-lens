// Command rnlens starts Lens, the React Native DevTools desktop application.
//
// Usage:
//
//	rnlens                 open the desktop app
//	rnlens targets         print discovered targets as JSON and exit
//	rnlens record ...      record the CDP traffic of a target to a JSONL file
//	rnlens --version       print the build version
package main

import (
	"fmt"
	"os"

	"github.com/Reidevl/rn-lens/frontend"
	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
	"github.com/Reidevl/rn-lens/internal/app"
)

// version is overridden at build time with -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			fmt.Println("rnlens " + version)
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

	core := app.New(app.Config{Version: version})
	core.Start()
	defer core.Stop()

	if err := wailsui.Run(core, frontend.Dist); err != nil {
		fmt.Fprintf(os.Stderr, "rnlens: %v\n", err)
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Print(`Lens - React Native DevTools

Usage:
  rnlens                 Open the desktop app
  rnlens targets         Print discovered targets as JSON and exit
  rnlens record          Record a target's CDP traffic to a JSONL file
  rnlens --version       Print the build version

Run "rnlens <command> -h" for command flags.
`)
}
