# Lens

**Lens** is an open-source React Native DevTools written in Go.

It attaches to any React Native app the same way React Native DevTools does and
adds what TanStack Query Devtools gives you on the web: live queries and
mutations, storage inspection, and an online/offline switch, in one fast desktop
window.

> Status: pre-alpha. The foundation is being built in the open. Nothing is
> published yet.

## What Lens will do

| Panel                                                                             | Source                             | Needs app code? |
| --------------------------------------------------------------------------------- | ---------------------------------- | --------------- |
| Console (logs, errors, object trees, eval REPL) — [guide](docs/guides/console.md) | Chrome DevTools Protocol via Metro | No              |
| Network (requests, bodies, timing, cURL, HAR)                                     | CDP (React Native 0.83+)           | No              |
| App and device metadata, reload, screenshot                                       | CDP                                | No              |
| Queries and Mutations (TanStack Query)                                            | `@rnlens/sdk`                      | One hook        |
| Storage (AsyncStorage, MMKV, SecureStore)                                         | `@rnlens/sdk`                      | One hook        |
| Performance traces (Perfetto export)                                              | CDP                                | No              |

## How it connects

Lens polls Metro for debuggable apps, attaches over the Chrome DevTools
Protocol as a first-class React Native DevTools client, and reconnects on its
own when Metro or the app restarts. The Targets panel shows what Metro
advertises, the app metadata React Native reports, and one-click reload or
hand-off to React Native DevTools. Details: [docs/guides/connect.md](docs/guides/connect.md).

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
```

See [CONTRIBUTING.md](CONTRIBUTING.md) once it lands.

## License

[MIT](LICENSE)
