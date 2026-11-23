# Lens

**Lens** is an open-source React Native DevTools written in Go.

It attaches to any React Native app the same way React Native DevTools does and
adds what TanStack Query Devtools gives you on the web: live queries and
mutations, storage inspection, and an online/offline switch, in one fast desktop
window.

> Status: early release (v0.1). Try it with `npx rnlens`, and please
> [report issues](https://github.com/Reidevl/rn-lens/issues) you find.

## What Lens does

| Panel                                                                                                                     | Source                             | Needs app code? |
| ------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- | --------------- |
| Console (logs, errors, object trees, eval REPL) — [guide](docs/guides/console.md)                                         | Chrome DevTools Protocol via Metro | No              |
| Network (requests, bodies, timing, cURL, HAR, JWT decoding) — [guide](docs/guides/network.md)                             | CDP (React Native 0.83+)           | No              |
| App and device metadata, reload — [guide](docs/guides/connect.md)                                                         | CDP                                | No              |
| Queries and Mutations (TanStack Query)                                                                                    | `@rnlens/sdk`                      | One hook        |
| Storage (AsyncStorage, MMKV, SecureStore, JWT decoding) — [guide](docs/guides/storage.md)                                 | `@rnlens/sdk`                      | One hook        |
| Plugins: your own panels, drawn by Lens — [guide](docs/guides/plugins.md)                                                 | `@rnlens/sdk`                      | One panel       |
| MCP: Claude and other AI clients read and manage the app — [guide](docs/guides/mcp.md)                                    | Lens                               | No              |
| Performance: screenshot, trace (Perfetto), CPU profile, heap snapshot, Fast Refresh — [guide](docs/guides/performance.md) | CDP                                | No              |

## Install

```sh
npx rnlens                                # Node.js 18+, nothing installed globally
brew install --cask reidevl/tap/rnlens    # macOS, Linux
scoop bucket add reidevl https://github.com/Reidevl/scoop-bucket
scoop install rnlens                      # Windows
```

These run Lens in your browser. The desktop app for macOS, Windows and Linux
is on [GitHub Releases](https://github.com/Reidevl/rn-lens/releases); it is
not notarized by Apple yet, so on macOS run
`xattr -dr com.apple.quarantine /Applications/Lens.app` once after moving it
to Applications. Every release has checksums, a cosign signature and build
provenance. See [docs/guides/install.md](docs/guides/install.md).

## How it connects

Lens polls Metro for debuggable apps, attaches over the Chrome DevTools
Protocol as a first-class React Native DevTools client, and reconnects on its
own when Metro or the app restarts. The Targets panel shows what Metro
advertises, the app metadata React Native reports, and one-click reload or
hand-off to React Native DevTools. Details: [docs/guides/connect.md](docs/guides/connect.md).

Browser mode: the server build (`task build:server`) runs Lens in a browser
tab instead of a window, on every OS. Details:
[docs/guides/browser-mode.md](docs/guides/browser-mode.md).

Headless helpers:

```bash
rnlens targets                 # list targets as JSON (exit 2 when none)
rnlens record -out cap.jsonl   # record a target's CDP traffic for fixtures
```

## Development

Requirements: Go 1.26+, Node 22+, pnpm 10+, [Task](https://taskfile.dev), and
the Wails v3 CLI (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`).

```bash
task dev     # run Lens with hot reload
task check   # lint, typecheck and test everything
task docs    # the documentation site (Astro Starlight) at localhost:4321
```

The site in `docs/site` is built from `docs/guides`, the protocol README and
the SDK README, plus its own use cases; edit the guides, not the synced pages.

See [CONTRIBUTING.md](CONTRIBUTING.md) to contribute and [GOVERNANCE.md](GOVERNANCE.md) for how collaborators are chosen.

## License

[MIT](LICENSE)
