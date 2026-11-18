// Package wailsui adapts the Lens core to the Wails v3 desktop shell.
//
// It is the only package allowed to import Wails. Every feature is exposed to
// the frontend as a Wails service; streams follow the notify-then-pull model
// described in ADR-0003.
package wailsui

import (
	"context"
	"io/fs"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

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
}

const (
	windowTitle  = "Lens"
	windowWidth  = 1280
	windowHeight = 800
	minWidth     = 960
	minHeight    = 600
)

// Run starts the desktop application and blocks until the window closes.
// assets is the embedded production frontend; in dev mode Wails serves the
// Vite dev server instead.
func Run(core *app.App, assets fs.FS) error {
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
	desktop = application.New(application.Options{
		Name:        windowTitle,
		Description: "React Native DevTools written in Go",
		Services: []application.Service{
			application.NewService(NewAppService(core)),
			application.NewService(NewTargetsService(core)),
			application.NewService(NewConsoleService(core)),
			application.NewService(NewQueriesService(core)),
			application.NewService(NewNetworkService(core, saveHAR)),
			application.NewService(NewStorageService(core, saveJSON, openJSON)),
			application.NewService(NewPerformanceService(core, saveAs)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
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
	topics := []string{app.TopicTargetsChanged, app.TopicSessionChanged, app.TopicConsoleChanged, app.TopicQueriesChanged, app.TopicNetworkChanged, app.TopicStorageChanged, app.TopicPerformanceChanged}
	coalescer := NewCoalescer(core.Bus(), topics, notifyTick,
		func(topic string, notice ChangeNotice) { desktop.Event.Emit(topic, notice) })
	go coalescer.Run(ctx)

	return desktop.Run()
}
