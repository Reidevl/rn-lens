# 0002. Two channels: CDP for the runtime, a dedicated WebSocket for the SDK

Date: 2026-11-17
Status: Accepted

## Context

React Native exposes its runtime through Metro's inspector proxy as a Chrome
DevTools Protocol endpoint (`ws://<metro>/inspector/debug?device=&page=`).
Console, exceptions, evaluation, network (RN 0.83+), reload, screenshots and
tracing are all reachable there with zero app code.

TanStack Query caches and app storage are not visible to CDP. Reading them
requires code running inside the app. Existing tools take two routes:

- Rozenite rides the CDP debugger connection through the private
  `__FUSEBOX_REACT_DEVTOOLS_DISPATCHER__` binding. It inherits the single
  debugger slot on React Native before 0.83 and its envelope is undocumented.
- Expo dev plugins and `react-query-external-sync` open their own WebSocket.

## Decision

Lens uses two channels:

1. **CDP channel.** Go is a CDP client of Metro. It sends
   `ReactNativeApplication.enable` before `Runtime.enable` so the app treats it
   as a first-class DevTools frontend.
2. **SDK channel.** `@rnlens/sdk` opens a WebSocket from the app to a server
   inside Lens (default port 7420; host derived from the Metro bundle URL so
   emulators and physical devices work unconfigured). Queries, mutations,
   storage and third-party plugins use this channel with a public, versioned
   JSON protocol (ADR-0004).

A CDP-binding transport for the SDK is **accepted but deferred**: it can be
added as a second `Transport` implementation for zero-config setups on targets
that allow multiple debuggers.

## Consequences

- Console and network work on any app without installing anything.
- Query and storage features need `npm i @rnlens/sdk` and one hook; the SDK is
  a no-op in production bundles.
- On React Native 0.76 to 0.82, attaching Lens evicts React Native DevTools
  (and vice versa). Lens shows an "exclusive attach" badge and documents it.
- We do not promise Rozenite plugin compatibility.
