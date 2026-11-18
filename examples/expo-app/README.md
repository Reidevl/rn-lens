# Lens example app

A minimal Expo app used to try Lens by hand and to record protocol fixtures.
It logs a line every two seconds, fetches JSON on demand, can throw an
uncaught error on demand, and streams its TanStack Query cache to Lens
through `@rnlens/sdk`.

It is intentionally **not** part of the pnpm workspace: it uses npm like most
React Native projects do.

## Run

```bash
npm run sdk:build       # builds ../../packages/sdk once (needs pnpm)
npm install
npx expo start          # press i for the iOS simulator, a for Android
```

Then open Lens (`task dev` from the repository root). The app appears in the
Targets panel within a couple of seconds, and in the Queries panel's device
list as soon as `useLens` connects.

`@rnlens/sdk` is a symlink to the workspace package; `metro.config.js` makes
Metro resolve the SDK's peers (React, React Native, TanStack Query) from this
app so only one copy of each is bundled. Rebuild the SDK after changing it.

## Queries and mutations

`App.tsx` mounts `useLens({ queryClient })` and exposes a `["todos"]` query,
a `["profile", { id: 1 }]` query and an `addTodo` mutation. The buttons let
you refetch, add a todo, or make the next fetch fail, so every query state
(fresh, stale, fetching, paused, inactive, error) can be seen in Lens.

Set `EXPO_PUBLIC_LENS_URL=ws://127.0.0.1:7421/lens` before `expo start` to
point the SDK at a Lens that is not listening on the default port 7420 (the
Go end-to-end test does this so it does not fight the Lens window).

## Storage

`storage.ts` exposes AsyncStorage, an MMKV instance (development build only;
Expo Go does not ship MMKV) and SecureStore (`token` declared) to Lens and
seeds a few values on launch: `theme`, `profile` (JSON) and an excluded
`internal:cache-version` in AsyncStorage; `launches`, `onboarded` and an
`avatar` byte array in MMKV. Open the Storage panel to browse and edit them.

## Network

Expo Go does not report network traffic, so build the app once as a
development build and run it from there:

```sh
npx expo run:ios      # or: npx expo run:android
```

Requests made with `XMLHttpRequest` (and axios) appear in the Network panel.
Expo SDK 58+ replaces `fetch` with `expo/fetch`, which no debugger sees; start
Metro with `EXPO_PUBLIC_USE_RN_FETCH=1` to use React Native's `fetch` instead.
See the [Network guide](../../docs/guides/network.md).

## Record fixtures

```bash
rnlens targets                                   # find the target id
rnlens record -target "<id>" -out ../../internal/adapters/cdpws/testdata/rn088-ios.jsonl -duration 20s
```
