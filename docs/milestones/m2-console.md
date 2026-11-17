# M2 Console

**Objective:** a live Console panel for the attached app: log levels, filters,
search, lazy object trees, stack traces for errors, and an eval REPL with
history.

## Scope

| Area     | Delivered                                                                                                                                                                                                    |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Protocol | Hand-written Runtime/Log vocabulary (remote objects, previews, stack traces, exceptions, log entries, context lifecycle) pinned by the recorded fixture                                                      |
| Core     | `internal/console`: bounded store with sequence numbers and epochs, CDP ingestion, Chrome-like argument formatting, domain enabling on attach, evaluate, lazy property expansion with stale-handle detection |
| Desktop  | `ConsoleService` (stats, range, clear, evaluate, properties) and the `console:changed` coalesced event                                                                                                       |
| Frontend | Console panel: notify-then-pull store, virtualized list that follows the tail, level chips with counts, text filter, clear, preserve log, lazy object trees, stack frames, eval bar with history             |
| Docs     | [Console guide](../guides/console.md)                                                                                                                                                                        |

## Acceptance checks

- [x] Attaching enables Runtime then Log; buffered lines replay
- [x] `console.log("tick", {...})` appears with an expandable object
- [x] Evaluate `1+1` prints `2`; thrown errors show as exception rows
- [x] A reload inserts a divider, marks earlier handles stale and clears the log unless Preserve log is on
- [x] Independent concurrency review of the service

## Out of scope

Network panel (M4), source-mapped locations, console groups and tables as structured views.
