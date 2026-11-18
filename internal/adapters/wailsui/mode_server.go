//go:build server

package wailsui

// browserMode is true in the server build (-tags server): Lens runs as a
// local web server and the UI opens in a browser tab.
const browserMode = true
