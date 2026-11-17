# M3 SDK + Queries

**Objective:** `npm i @rnlens/sdk` + `useLens({ queryClient })` gives Lens live
Queries and Mutations panels with every action the official TanStack Query
devtools offer, plus an online/offline switch.

## Scope

| Area     | Delivered                                                                                                                                                                                                     |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Protocol | JSON Schema v1 under `protocol/` with fixtures, hand-written Go types proven by strict decoding, generated TypeScript types checked with Ajv                                                                  |
| Server   | `internal/adapters/sdkws`: WebSocket endpoint on port 7420 with hello/welcome, protocol-major and token checks, one session per app and device, liveness pings                                                |
| Core     | `internal/queries`: per-device mirror with the delta policy, devtools status labels and ordering, actions with correlated results, online flag                                                                |
| SDK      | `@rnlens/sdk`: `useLens` / `createLensClient`, host derived from Metro, reconnect with backoff, no-op in production, TanStack adapter with the official action recipes, `onlineManager` sync, safe serializer |
| Desktop  | `QueriesService` and the `queries:changed` event; `rnlens --sdk-port` / `--sdk-token`                                                                                                                         |
| Frontend | Queries panel (device picker, online switch, status chips, filter, detail with actions, JSON tree, data editor) and Mutations panel                                                                           |
| Docs     | [Queries guide](../guides/queries.md), SDK README, protocol README                                                                                                                                            |

## Acceptance checks

- [ ] The example app appears in Queries within a second of starting
- [ ] A query shows as fresh, turns stale on Invalidate and fetching on Refetch
- [ ] Edited data reaches the app (`queryClient.getQueryData` returns it)
- [ ] Offline switches `onlineManager.isOnline()` on the device
- [ ] Independent review of the server, mirror and SDK

## Out of scope

Storage (M5), plugins (M7), request mocking.
