// Command rnlens starts Lens, the React Native DevTools desktop application.
package main

import (
	"log"
	"os"

	"github.com/Reidevl/rn-lens/frontend"
	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
	"github.com/Reidevl/rn-lens/internal/app"
)

// version is overridden at build time with -ldflags "-X main.version=<semver>".
var version = "dev"

func main() {
	core := app.New(app.Config{Version: version})

	if err := wailsui.Run(core, frontend.Dist); err != nil {
		log.Printf("rnlens: %v", err)
		os.Exit(1)
	}
}
