# 0003. Notify-then-pull between Go and the UI

Date: 2026-11-17
Status: Accepted

## Context

Wails v3 delivers each Go-emitted event by serializing it to JSON and executing
a JavaScript call on the webview's UI thread. A console stream of a few thousand
lines per second pushed as individual events would saturate that thread and the
UI would stutter. Network bodies and large console objects are also too big to
ride events.

## Decision

Go owns every stream in a bounded ring buffer (defaults: 10,000 console
entries, 2,000 network requests, byte-capped bodies with least-recently-used
eviction). The Wails adapter emits at most one small `<feature>:changed`
notification per 16 ms tick carrying the latest sequence number and a dropped
count. The UI pulls only the visible range through a service call
(`GetRange(fromSeq, limit)`), driven by the list virtualizer. Payloads never
ride events.

## Consequences

- Rendering cost is bounded by what is on screen, not by the event rate.
- Memory is bounded and predictable; the UI shows when entries were dropped.
- Remote object handles and response bodies are fetched lazily on expand.
- The `Backend` interface models this explicitly, so a WebSocket transport for
  a browser or headless mode follows the same contract.
