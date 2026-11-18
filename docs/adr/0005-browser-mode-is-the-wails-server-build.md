# 0005. Browser mode is the Wails server build

Date: 2026-11-18
Status: Accepted

## Context

The plan called for `rnlens serve`: Lens in a browser tab, for machines
without a desktop session, for teammates who prefer a tab, and for an `npx`
launcher that works on every OS. It also called for a WebSocket `Backend` so
the frontend would not depend on Wails. Wails v3.1 ships a server mode
(`-tags server`) that serves the same frontend and service bindings over HTTP,
with events over a WebSocket, a host guard against DNS rebinding, and no
native dependencies: it builds with `CGO_ENABLED=0` for Linux, Windows and
macOS from one machine. The desktop build needs cgo per OS. Server mode has no
native file dialogs.

## Decision

Browser mode is the server build of the same `cmd/rnlens`, not a second
transport. The binary built with `-tags server` serves the UI on
`localhost:7430` and opens the browser; the desktop build opens a window. The
frontend keeps the Wails adapter and learns the mode from `/lens/mode`. Saves
become one-time browser downloads and imports start with an upload
(`/lens/downloads/<token>/<name>`, `/lens/uploads`), served from a private
temporary directory. The clipboard is the browser's.

## Consequences

- One service layer and one frontend adapter serve both shells; no second
  RPC protocol to keep in sync.
- The server build is the portable binary: release archives and the `npx`
  launcher can ship it for every OS without per-OS cgo toolchains.
- `rnlens serve` is not a subcommand of the desktop binary; the server build
  runs browser mode by default.
- Listening beyond localhost exposes the app's data to that network; the CLI
  warns when `--host` is not a loopback address.
- The frontend still depends on Wails' JavaScript runtime. A Lens-owned
  WebSocket transport stays possible behind the `Backend` interface if Wails
  server mode ever falls short.
