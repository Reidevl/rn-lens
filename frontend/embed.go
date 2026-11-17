// Package frontend embeds the production build of the Lens web UI.
//
// Run `task common:build:frontend` to populate dist before building the Go
// binary; dist/.gitkeep keeps the embed pattern valid for test-only builds.
package frontend

import "embed"

// Dist holds the compiled frontend under the dist/ prefix.
//
//go:embed all:dist
var Dist embed.FS
