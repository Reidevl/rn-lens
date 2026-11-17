# M0 Foundation

**Objective:** a repository where `task dev` opens an empty Lens window with the
design system and `task check` is green locally and in CI.

## Scope

| Area          | Delivered                                                                                                                                                                                         |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository    | Go module, pnpm workspace, MIT license, editor and git configuration, Task-based workflow                                                                                                         |
| Desktop shell | Wails v3 bootstrap; `cmd/rnlens` entry point; `internal/app` composition root without Wails imports; `internal/adapters/wailsui` as the only Wails-aware package                                  |
| Frontend      | Vite 8, React 19, Tailwind 4, Vitest, ESLint; design tokens with dark and light themes; left rail, status bar and placeholder panels; the `Backend` interface with Wails and mock implementations |
| Quality gates | golangci-lint v2, gotestsum, `task check`                                                                                                                                                         |
| Automation    | CI matrix on macOS, Ubuntu and Windows; release-please; Renovate; CODEOWNERS; issue forms; PR template                                                                                            |
| Governance    | CONTRIBUTING, Code of Conduct, SECURITY threat model, ADR-0001 to ADR-0004                                                                                                                        |

## Acceptance checks

- [x] `task check` passes (Go tests, Vitest, lint, typecheck)
- [x] `task build` produces `bin/rnlens` and the binary runs
- [x] `actionlint` reports no issues
- [x] Independent review of the CI and release configuration

## Out of scope

Any connection to a React Native app (M1), panels with real data (M2+).
