# Contributing to Lens

Thanks for helping build a React Native DevTools the community can rely on.
This guide is short on purpose: the quick path first, details after.

Pull requests are open to collaborators only. If you are not one yet, start
with an issue; [GOVERNANCE.md](GOVERNANCE.md) explains how collaborators are
chosen.

## Quick path

1. Clone the repository, then install the toolchain: Go 1.26+, Node 22+, pnpm 10+,
   [Task](https://taskfile.dev), and `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`.
2. `pnpm install && task dev` opens Lens with hot reload.
3. Create a branch from `main`: `git switch -c feat/<area>-<topic>`.
4. Make one change, with its tests and docs, and run `task check`.
5. Open a pull request. CI must be green and the diff should be about 400
   authored lines or less.

## Project shape

| Path                  | What lives there                                                                                                      |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `cmd/rnlens`          | Entry point only                                                                                                      |
| `internal/app`        | Composition root; imports no UI framework                                                                             |
| `internal/<feature>`  | One package per feature (targets, console, network, queries, storage, tracing); each owns its domain, ports and store |
| `internal/platform`   | Generic building blocks (ring buffers, ids, bus)                                                                      |
| `internal/adapters/*` | The outside world: CDP client, Metro discovery, SDK WebSocket server, Wails UI                                        |
| `protocol/`           | JSON Schema for the SDK protocol and its fixtures                                                                     |
| `frontend/`           | Desktop UI (React, Vite, Tailwind). Panels only talk to Go through `src/backend`                                      |
| `packages/sdk`        | `@rnlens/sdk`, the in-app package                                                                                     |
| `docs/`               | Documentation site and Architecture Decision Records                                                                  |

Rules that keep the architecture honest:

- Feature packages import `internal/platform` and their own ports, never an adapter.
- Only `internal/adapters/wailsui` may import Wails.
- Frontend features never import `@wailsio/runtime` or the generated bindings; they use the `Backend` interface.
- Streams are notify-then-pull (ADR-0003): Go keeps bounded buffers, emits tiny change notifications, the UI pulls ranges.

## Commits and pull requests

- Use [Conventional Commits](https://www.conventionalcommits.org): `feat(console): ...`, `fix(cdp): ...`, `docs: ...`. The scope is the feature or adapter package.
- One commit is one work unit: a behavior, fix or doc change together with its tests. Do not split "code", "tests" and "docs" into separate commits.
- Keep PRs reviewable: about 400 authored changed lines. Bigger changes become a chain of PRs, each one green on its own.
- Do not add AI attribution trailers to commits.
- PRs are merged into `main` with a merge commit, so keep each commit in the PR clean and Conventional. `main` accepts changes only through pull requests.

## Testing

| Layer      | Command              | Notes                                                                     |
| ---------- | -------------------- | ------------------------------------------------------------------------- |
| Go         | `task test:go`       | Table-driven tests, `t.Parallel()` where safe, fakes over mocks           |
| Frontend   | `task test:frontend` | Vitest + Testing Library, render through `App` with `createMockBackend()` |
| Everything | `task check`         | Lint, typecheck and tests, the same as CI                                 |

Write the failing test first when a deterministic test exists for the behavior.
Keep scratch files out of the tree (`t.TempDir()` in Go).

## Documentation

User-visible changes ship with their docs in the same PR. Architecture decisions
get an ADR in `docs/adr` (see its README for the template).

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
