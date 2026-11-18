//go:build !server

package wailsui

// browserMode is false in the desktop build: Lens runs in a native window.
const browserMode = false
