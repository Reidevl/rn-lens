# M7 Plugins, docs, distribution

**Objective:** third-party panels through a plugin API, a documentation site
with use cases, installable releases (`npx rnlens`, Homebrew, Scoop, GitHub
Releases with checksums, cosign signatures and provenance), and a browser
mode.

## Scope

| Area         | Delivered                                                                                                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Browser mode | The server build (`-tags server`, no cgo) serves Lens on `localhost:7430`; downloads and uploads replace native dialogs ([ADR-0005](../adr/0005-browser-mode-is-the-wails-server-build.md)) |
| Plugins      | Protocol additive minor (`plugin.register`, `plugin.view`, `plugin.action`, `plugin.result`), `internal/plugins`, `createLensPanel` and `useLens({ panels })`, the Plugins panel            |
| Distribution | GoReleaser (6 archives, checksums, Homebrew cask, Scoop), the `rnlens` npm launcher with per-platform packages, a release workflow (cosign, SBOMs, provenance, npm, desktop apps)           |
| Docs         | [Install](../guides/install.md), [Browser mode](../guides/browser-mode.md), [Plugins](../guides/plugins.md) guides; the Starlight site in `docs/site` with five use cases                   |

## Acceptance checks

- [x] `goreleaser release --snapshot` produces all archives (darwin, linux, windows × amd64, arm64), `checksums.txt`, the cask and the Scoop manifest
- [x] `npx rnlens` launches: the npm packages built from the snapshot, installed from local packs, serve Lens in browser mode and stop on Ctrl+C
- [x] Browser mode in headless Chrome against an RN 0.88.1 development build: attach, screenshot and storage export downloads, storage import upload
- [x] A plugin panel end to end on the same build: the example's Feature flags table, a row action with its message, a confirmed reset, the log
- [x] The docs site builds with every internal link resolving
- [ ] The release workflow runs (GitHub Actions is disabled on this repository; checked with `actionlint` only)
- [ ] First release published (needs the maintainer: the tap and bucket repositories, `HOMEBREW_TAP_TOKEN`, `SCOOP_BUCKET_TOKEN`, `NPM_TOKEN`)
- [ ] Apple notarization (needs a Developer ID; the install guide documents `xattr` until then)

## Out of scope

Plugin panels with their own UI code (the declarative model was chosen;
sandboxed web panels can come later), hosting the docs site, and a Lens
auto-updater.
