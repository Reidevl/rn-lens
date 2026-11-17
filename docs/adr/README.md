# Architecture Decision Records

Decisions that shape Lens, one file each, in [MADR](https://adr.github.io/madr/) style.
An ADR is immutable once accepted; a new ADR supersedes it.

| ADR                                                 | Decision                                                             | Status   |
| --------------------------------------------------- | -------------------------------------------------------------------- | -------- |
| [0001](0001-go-core-with-wails-desktop-shell.md)    | Go core with a Wails v3 desktop shell                                | Accepted |
| [0002](0002-two-channels-cdp-and-sdk-websocket.md)  | Two channels: CDP for the runtime, a dedicated WebSocket for the SDK | Accepted |
| [0003](0003-notify-then-pull-frontend-data-path.md) | Notify-then-pull between Go and the UI                               | Accepted |
| [0004](0004-schema-first-versioned-sdk-protocol.md) | Schema-first, versioned SDK protocol with contract tests             | Accepted |

## Template

```markdown
# NNNN. Title

Date: YYYY-MM-DD
Status: Proposed | Accepted | Superseded by NNNN

## Context

## Decision

## Consequences
```
