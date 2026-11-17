# M1 Connect

**Objective:** Lens lists running React Native apps from Metro, attaches as a
first-class React Native DevTools client, shows app and device metadata,
survives reloads and Metro restarts, and can reload the app or hand off to
React Native DevTools.

## Scope

| Area          | Delivered                                                                                                                                                  |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Platform      | Bounded, sequence-numbered ring buffer; non-blocking notification bus; clock port                                                                          |
| CDP transport | JSON-RPC client over WebSocket with correlated calls, ordered events, typed errors and parsed close reasons; an in-process fake inspector proxy for tests  |
| Metro         | `/json` discovery across several endpoints with diffs and backoff; `/open-debugger` hand-off                                                               |
| Targets       | Target model with capabilities; attach session state machine (handshake, eviction, reconnect with backoff, re-resolution of the page after an app restart) |
| CLI           | `rnlens targets` (JSON) and `rnlens record` (CDP capture to JSONL); example Expo app                                                                       |
| Desktop       | Targets service with coalesced change events; Targets panel; live session status in the status bar                                                         |
| Docs          | [How Lens connects](../guides/connect.md); opt-in end-to-end test against a live app                                                                       |

## Acceptance checks

- [x] The example app appears in Targets within two seconds of starting Metro
- [x] Attaching shows app name, platform and React Native version
- [x] Reload keeps the session attached
- [x] Killing and restarting Metro brings the session back without user action
- [x] Another debugger taking the page is reported as an eviction with "Attach again"
- [x] Independent concurrency review of the session and transport

## Out of scope

Console, network and any in-app SDK (M2 onward).
