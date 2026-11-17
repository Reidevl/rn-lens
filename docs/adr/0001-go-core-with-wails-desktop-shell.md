# 0001. Go core with a Wails v3 desktop shell

Date: 2026-11-17
Status: Accepted

## Context

Lens must hold thousands of console lines and network requests per second
without stalling, speak WebSocket-based protocols to Metro and to an in-app SDK,
and ship as a single installable artifact to macOS, Windows and Linux. The UI
needs DevTools-class widgets (virtualized logs, JSON trees, resizable tables)
that browser engines render well and native toolkits do not.

Alternatives considered:

- Pure-Go UI (Fyne, Gio): no rich text selection, no mature virtualized tree,
  every DevTools widget would be hand-built.
- Go HTTP server plus the system browser (pprof model): zero native
  dependencies, but no native window, tray, menus or updater, and a "paste this
  URL" experience.
- Electron or Tauri: a second language and build system in a Go-first project.

## Decision

Lens is a Go program. The UI is a web application (React, Vite, Tailwind)
embedded into the binary and shown by **Wails v3** (stable since v3.0.0,
2026-10-26). Go exposes features as Wails services with generated TypeScript
bindings; the UI runs in the OS webview.

The composition root `internal/app` imports no Wails package, and only
`internal/adapters/wailsui` may. The frontend reaches Go exclusively through a
`Backend` interface so a browser or headless transport can be added later.

## Consequences

- One binary per platform, around 10 MB, using the system webview (WebView2 on
  Windows, WebKitGTK 6.0 on Linux, WebKit on macOS).
- macOS distribution eventually needs Developer ID signing and notarization;
  local builds run unsigned, so this is deferred until the first public release.
- Wails v3 is young; we pin the framework and its JS runtime together and
  isolate it behind one adapter.
