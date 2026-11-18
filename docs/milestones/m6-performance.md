# M6 Performance

**Objective:** a Performance panel to take screenshots, record traces for
Perfetto UI and Chrome DevTools, CPU profiles and Hermes heap snapshots of
the attached app, plus a Fast Refresh indicator.

## Scope

| Area     | Delivered                                                                                                                                         |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| CDP      | `Page.captureScreenshot`, `Tracing.start`/`end` with `categories` and chunk collection until `tracingComplete`, `Profiler`, `HeapProfiler` chunks |
| Core     | `internal/perf` (one recording of each kind at a time, reset on re-attach, Chrome JSON export, summaries); `internal/fastrefresh` (Metro `/hot`)  |
| Desktop  | `PerformanceService` with native save dialogs (heap snapshots stream to disk) and the `performance:changed` event                                 |
| Frontend | Performance panel (screenshot, trace, CPU profile, heap snapshot cards) and the Fast Refresh status bar item                                      |
| Docs     | [Performance guide](../guides/performance.md)                                                                                                     |

## Acceptance checks

- [x] A recorded trace opens in Perfetto (`TestE2E_PerformanceRecordings` writes it with `LENS_E2E_TRACE_OUT`; Perfetto's trace processor v59.1 loads it with the user timing marks)
- [x] The screenshot is a PNG, the profile has nodes and the heap snapshot parses (same test, green on an RN 0.88.1 development build)
- [x] The status bar follows Fast Refresh: the same test edits a source file (`LENS_E2E_FAST_REFRESH_FILE`) and sees the update; after a Metro restart it shows the reload hint and registers again once the app reloads. Bundling errors are covered by unit tests
- [ ] Independent review of the recording concurrency and the new desktop surface

## Out of scope

Viewing traces inside Lens (Perfetto UI and Chrome DevTools do it better),
memory sampling (`HeapProfiler.startSampling`), allocation timelines, and
performance overlays on the device.
