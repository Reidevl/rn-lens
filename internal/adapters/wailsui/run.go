// Package wailsui adapts the Lens core to the Wails v3 desktop shell.
//
// It is the only package allowed to import Wails. Every feature is exposed to
// the frontend as a Wails service; streams follow the notify-then-pull model
// described in ADR-0003.
package wailsui

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/Reidevl/rn-lens/internal/adapters/mcpserver"
	"github.com/Reidevl/rn-lens/internal/app"
)

// notifyTick bounds how often a changed topic reaches the webview.
const notifyTick = 16 * time.Millisecond

func init() {
	// Registering the events gives the bindings generator typed TS signatures.
	application.RegisterEvent[ChangeNotice](app.TopicTargetsChanged)
	application.RegisterEvent[ChangeNotice](app.TopicSessionChanged)
	application.RegisterEvent[ChangeNotice](app.TopicConsoleChanged)
	application.RegisterEvent[ChangeNotice](app.TopicQueriesChanged)
	application.RegisterEvent[ChangeNotice](app.TopicNetworkChanged)
	application.RegisterEvent[ChangeNotice](app.TopicStorageChanged)
	application.RegisterEvent[ChangeNotice](app.TopicPerformanceChanged)
	application.RegisterEvent[ChangeNotice](app.TopicPluginsChanged)
	application.RegisterEvent[ChangeNotice](app.TopicMCPChanged)
}

const (
	windowTitle  = "Lens"
	windowWidth  = 1280
	windowHeight = 800
	minWidth     = 960
	minHeight    = 600
)

// RunOptions configures Run.
type RunOptions struct {
	// Host and Port are where browser mode (the server build) listens.
	// Ignored by the desktop build.
	Host string
	Port int
	// MCP is the running MCP endpoint, shown in the UI; empty when off.
	MCP MCPInfo
	// MCPControl is the endpoint's activity log and read-only switch; nil
	// when MCP is off.
	MCPControl *mcpserver.Control
}

// Run starts Lens and blocks until it ends: a native window in the desktop
// build, a local web server in the server build (browser mode). assets is
// the embedded production frontend; in dev mode Wails serves the Vite dev
// server instead.
func Run(core *app.App, assets fs.FS, opts RunOptions) error {
	var desktop *application.App
	saveHAR := func(name string) (string, error) {
		return desktop.Dialog.SaveFile().
			SetFilename(name).
			AddFilter("HAR archive", "*.har").
			CanCreateDirectories(true).
			PromptForSingleSelection()
	}
	saveJSON := func(name string) (string, error) {
		return desktop.Dialog.SaveFile().
			SetFilename(name).
			AddFilter("JSON", "*.json").
			CanCreateDirectories(true).
			PromptForSingleSelection()
	}
	saveAs := func(name, label, pattern string) (string, error) {
		return desktop.Dialog.SaveFile().
			SetFilename(name).
			AddFilter(label, pattern).
			CanCreateDirectories(true).
			PromptForSingleSelection()
	}
	openJSON := func(string) (string, error) {
		return desktop.Dialog.OpenFile().
			CanChooseFiles(true).
			AddFilter("Lens storage export", "*.json").
			PromptForSingleSelection()
	}
	appService := NewAppService(core)
	appService.mcp = opts.MCP
	storageService := NewStorageService(core, saveJSON, openJSON)
	lensRoutes := http.NotFoundHandler()
	if browserMode {
		// No native dialogs in a browser: saves become downloads and
		// imports arrive as uploads (see transfers).
		tr, err := newTransfers()
		if err != nil {
			return err
		}
		defer tr.close()
		saveHAR, saveJSON = tr.savePath, tr.savePath
		saveAs = func(name, _, _ string) (string, error) { return tr.savePath(name) }
		openJSON = func(string) (string, error) {
			return "", errors.New("lens: in the browser, pick the file to upload instead")
		}
		appService.transfers = tr
		storageService = NewStorageService(core, saveJSON, openJSON)
		storageService.transfers = tr
		lensRoutes = tr
	}
	desktop = application.New(application.Options{
		Name:        windowTitle,
		Description: "React Native DevTools written in Go",
		Services: []application.Service{
			application.NewService(appService),
			application.NewService(NewTargetsService(core)),
			application.NewService(NewConsoleService(core)),
			application.NewService(NewQueriesService(core)),
			application.NewService(NewNetworkService(core, saveHAR)),
			application.NewService(storageService),
			application.NewService(NewPerformanceService(core, saveAs)),
			application.NewService(NewPluginsService(core)),
			application.NewService(NewMCPService(opts.MCP, opts.MCPControl)),
		},
		Assets: application.AssetOptions{
			Handler: withLensRoutes(lensRoutes, application.AssetFileServerFS(assets)),
		},
		Server: application.ServerOptions{Host: opts.Host, Port: opts.Port},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     windowTitle,
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  minWidth,
		MinHeight: minHeight,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		// Matches --color-background so the window never flashes white.
		BackgroundColour: application.NewRGB(15, 23, 42),
		URL:              "/",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	topics := []string{app.TopicTargetsChanged, app.TopicSessionChanged, app.TopicConsoleChanged, app.TopicQueriesChanged, app.TopicNetworkChanged, app.TopicStorageChanged, app.TopicPerformanceChanged, app.TopicPluginsChanged, app.TopicMCPChanged}
	coalescer := NewCoalescer(core.Bus(), topics, notifyTick,
		func(topic string, notice ChangeNotice) { desktop.Event.Emit(topic, notice) })
	go coalescer.Run(ctx)

	return desktop.Run()
}

// withLensRoutes serves /lens/mode (which tells the frontend whether it runs
// in a browser) and hands the rest of /lens/ to lens, everything else to
// assets.
func withLensRoutes(lens, assets http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lens/mode":
			mode := "desktop"
			if browserMode {
				mode = "browser"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(`{"mode":"` + mode + `"}`))
		case strings.HasPrefix(r.URL.Path, "/lens/"):
			lens.ServeHTTP(w, r)
		default:
			assets.ServeHTTP(w, r)
		}
	})
}
