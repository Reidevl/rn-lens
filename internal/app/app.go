// Package app is the composition root of Lens.
//
// It wires feature services together without importing any UI framework, so
// the same core runs under the Wails desktop shell, a headless CLI, or tests.
package app

// Config holds the values the composition root needs from its caller.
type Config struct {
	// Version is the build version shown in the UI ("dev" for local builds).
	Version string
}

// Info describes the running Lens instance.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// App owns every feature service for the lifetime of the process.
type App struct {
	cfg Config
}

// New builds the application graph from cfg. It performs no I/O.
func New(cfg Config) *App {
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	return &App{cfg: cfg}
}

// Info returns static information about this Lens build.
func (a *App) Info() Info {
	return Info{Name: "Lens", Version: a.cfg.Version}
}
