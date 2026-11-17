# Lens

**Lens** is an open-source React Native DevTools written in Go.

It attaches to any React Native app the same way React Native DevTools does and
adds what TanStack Query Devtools gives you on the web: live queries and
mutations, storage inspection, and an online/offline switch, in one fast desktop
window.

> Status: pre-alpha. The foundation is being built in the open. Nothing is
> published yet.

## What Lens will do

| Panel | Source | Needs app code? |
|---|---|---|
| Console (logs, errors, eval REPL) | Chrome DevTools Protocol via Metro | No |
| Network (requests, bodies, timing, cURL, HAR) | CDP (React Native 0.83+) | No |
| App and device metadata, reload, screenshot | CDP | No |
| Queries and Mutations (TanStack Query) | `@rnlens/sdk` | One hook |
| Storage (AsyncStorage, MMKV, SecureStore) | `@rnlens/sdk` | One hook |
| Performance traces (Perfetto export) | CDP | No |

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
