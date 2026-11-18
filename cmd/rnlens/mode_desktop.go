//go:build !server

package main

import (
	"flag"

	"github.com/Reidevl/rn-lens/internal/adapters/wailsui"
)

const runUsage = `  rnlens                 Open the desktop app
`

// modeFlags has no flags in the desktop build: the window needs no address.
type modeFlags struct{}

func registerModeFlags(*flag.FlagSet) modeFlags { return modeFlags{} }

func (modeFlags) options() wailsui.RunOptions { return wailsui.RunOptions{} }

func (modeFlags) started() {}
