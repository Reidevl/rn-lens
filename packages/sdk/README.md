# @rnlens/sdk

React Native SDK for [Lens](../../README.md). It streams your TanStack Query
cache to the Lens desktop app and runs the devtools actions Lens asks for
(refetch, invalidate, trigger loading or error, set data, clear caches) on the
device. Production bundles get a no-op: no socket is ever opened.

## Install

```bash
npm install @rnlens/sdk
# or
pnpm add @rnlens/sdk
```

Peer dependencies: `react` (>= 18). `react-native` (>= 0.76) and
`@tanstack/react-query` (>= 5) are optional: without a `queryClient` the SDK
only announces the device to Lens.

## Use

Expo or bare React Native, with TanStack Query:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLens } from "@rnlens/sdk";

const queryClient = new QueryClient();

export default function App() {
  useLens({ queryClient });
  return <QueryClientProvider client={queryClient}>{/* your app */}</QueryClientProvider>;
}
```

`useLens` creates one client for the component's lifetime, connects on mount,
disconnects on unmount and returns the connection state
(`"idle" | "connecting" | "connected" | "reconnecting" | "disabled"`).
`disabled` is the state of a production (no-op) client and of a client that
stopped after a fatal close code (see Troubleshooting); `connect()` retries.

Outside React, or when you need the client itself:

```ts
import { createLensClient } from "@rnlens/sdk";

const lens = createLensClient({ queryClient, logger: console });
lens.connect();
lens.onStateChange((state) => console.log(state));
// later
lens.disconnect();
```

## Options

| Option          | Type                   | Default                                     | Purpose                                                                |
| --------------- | ---------------------- | ------------------------------------------- | ---------------------------------------------------------------------- |
| `queryClient`   | `QueryClient`          | —                                           | TanStack client to mirror; enables the Queries and Mutations panels.   |
| `storage`       | `LensStorageOptions`   | —                                           | Storage instances to mirror; enables the Storage panel (see Storage).  |
| `panels`        | `LensPanel[]`          | —                                           | Panels Lens draws in its Plugins panel (see Panels).                   |
| `url`           | `string`               | derived, see below                          | Full WebSocket URL of Lens.                                            |
| `token`         | `string`               | —                                           | Pairing token when Lens runs with `--sdk-token`.                       |
| `deviceName`    | `string`               | device model                                | Name shown in the Lens device list.                                    |
| `appName`       | `string`               | `appId`                                     | App name shown in Lens.                                                |
| `appId`         | `string`               | `appName` or `"app"`                        | Bundle id or package name; sessions are keyed by app id and device id. |
| `enabled`       | `boolean`              | `__DEV__`, else `NODE_ENV !== "production"` | When false every method is a no-op and no socket is opened.            |
| `logger`        | `LensLogger`           | silent                                      | Pass `console` to see connection diagnostics.                          |
| `WebSocketImpl` | `WebSocketConstructor` | global `WebSocket`                          | Alternative WebSocket implementation (tests, custom runtimes).         |
| `backoff`       | `{ minMs?, maxMs? }`   | `{ minMs: 500, maxMs: 10000 }`              | Reconnect backoff bounds.                                              |

## Storage

Pass the storage instances the app already uses; the SDK imports none of
these libraries and only relies on the methods listed in its exported
`AsyncStorageLike`, `MMKVLike` (react-native-mmkv v3 `new MMKV()` or v4
`createMMKV()`) and `SecureStoreLike` (expo-secure-store) types.

```ts
import AsyncStorage from "@react-native-async-storage/async-storage";
import * as SecureStore from "expo-secure-store";
import { createMMKV } from "react-native-mmkv"; // v3: new MMKV()

const storage = createMMKV();

useLens({
  queryClient,
  storage: {
    asyncStorage: AsyncStorage,
    mmkv: { default: storage },
    secureStore: { module: SecureStore, keys: ["session.token"] },
    exclude: ["pin", /^auth\./],
  },
});
```

| Store        | Id                  | Listing                                     | Changes                                     |
| ------------ | ------------------- | ------------------------------------------- | ------------------------------------------- |
| AsyncStorage | `async-storage`     | every key; sizes when the store has ≤ 500   | snapshot of the store after each Lens write |
| MMKV         | `mmkv:<record key>` | every key; sizes of string and bytes values | live, through `addOnValueChangedListener`   |
| SecureStore  | `secure-store`      | only the declared `keys`; no sizes          | snapshot of the store after each Lens write |

- Snapshots carry keys only; Lens reads a value with `GET` when it is opened.
  A `storage.snapshot` is sent after `welcome`, on `REFRESH` (that store only)
  and after every `SET`, `REMOVE` or `CLEAR` on AsyncStorage or SecureStore,
  which have no change listener. AsyncStorage changes made by the app itself
  show up on the next refresh.
- MMKV reports every change, from the app or from Lens, as `storage.event`
  (`set` with the size, `removed`); a `CLEAR` from Lens sends one `cleared`.
- `exclude` hides keys: a string matches a key exactly, a RegExp is tested
  against it. Excluded keys never appear in snapshots or events, every action
  on them is answered `ok: false` with `"key is excluded by the app"`, and
  `CLEAR` leaves them in place.
- SecureStore cannot enumerate keys: only `secureStore.keys` are listed, and
  actions on other keys are refused. A key deleted from Lens leaves the list
  until Lens sets it again, a `REFRESH`, or an app reload. `CLEAR` deletes the
  declared keys that are not excluded.
- Values: AsyncStorage and SecureStore store strings and refuse other types.
  MMKV stores strings, numbers, booleans and bytes (base64 on the wire). MMKV
  does not expose an entry's type, so `GET` tries `getString`, `getNumber`,
  `getBoolean` and `getBuffer` in that order and the first defined answer wins.
- Limits: a value whose text form exceeds 1 MiB (`STORAGE_VALUE_MAX_BYTES`) is
  answered with `truncated: true`, its `size` and an empty `value`. A store
  lists at most 10,000 keys (`STORAGE_SNAPSHOT_MAX_KEYS`), sorted, and sets
  `truncated` when it has more.
- Every `storage.action` is answered with one `storage.result`
  (`{ id, ok, error?, value? }`); a missing key reads as `ok: false, "not set"`.

## Panels

A panel adds your own tab to Lens's Plugins panel: feature flags, a Redux or
Zustand store, a job queue, anything the app knows. You describe it with
building blocks and Lens draws it; no UI code of yours runs in Lens, and it
works the same in the desktop app and in browser mode.

```tsx
import { createLensPanel, useLens } from "@rnlens/sdk";

const flags = { newCheckout: true, darkMode: false };

const flagsPanel = createLensPanel({
  id: "feature-flags",
  name: "Feature flags",
  render: () => [
    {
      type: "table",
      id: "flags",
      columns: [
        { key: "name", label: "Flag" },
        { key: "on", label: "On" },
      ],
      rows: Object.entries(flags).map(([name, on]) => ({ id: name, cells: { name, on } })),
      rowActions: [{ id: "toggle", label: "Toggle" }],
    },
  ],
  onAction: ({ rowId }) => {
    flags[rowId as keyof typeof flags] = !flags[rowId as keyof typeof flags];
    return `${rowId} toggled`;
  },
});

export function App() {
  useLens({ panels: [flagsPanel] });
  // …your app
}
```

| Block      | Fields                                                              |
| ---------- | ------------------------------------------------------------------- |
| `text`     | `text`, `tone?` (`default`, `muted`, `warning`, `error`)            |
| `keyValue` | `items: [{ key, label?, value }]`                                   |
| `table`    | `columns: [{ key, label }]`, `rows: [{ id, cells }]`, `rowActions?` |
| `json`     | `value` (any JSON value, shown as a tree)                           |
| `log`      | `entries: [{ text, level?, time? }]`, newest last                   |
| `actions`  | `actions: [{ id, label, destructive?, confirm? }]`                  |

`useLens` reads its options once: create panels at module level and pass
every panel on the first render. A panel needs a `name` of 1 to 64
characters and an `id` of lowercase letters, digits, `.`, `_` or `-`.

Every block has an `id` and an optional `title`. Actions call `onAction` with
`{ blockId, actionId, rowId? }`: return a short message to show in Lens, or
throw to report a failure. The panel re-renders after each action; call
`panel.refresh()` when its data changes for another reason (refreshes within
16 ms send one view). Values are sent like query data (BigInt and Date as
strings). A view larger than 1 MiB (`PANEL_VIEW_MAX_BYTES`) or a `render` that
throws shows an error block instead.

## How the URL is derived

When `url` is not set the SDK connects to `ws://<host>:7420/lens`, where
`<host>` is, in order:

1. the host of the Metro bundle URL the app loaded (`NativeModules.SourceCode.scriptURL`),
   so a physical device on the LAN reaches the machine that runs Metro;
2. `location.hostname` in a browser;
3. `localhost`.

On Android, `localhost` and `127.0.0.1` become `10.0.2.2`, the emulator's
alias for the host machine.

## What is sent

- `hello` with the SDK version, protocol version 1, app and device identity,
  the Metro script URL and the feature list (`"queries"` when a
  `queryClient` is given, `"storage"` when `storage` is given, `"plugins"` when `panels` are given). The device id is persisted with
  `@react-native-async-storage/async-storage` when that package is installed,
  otherwise it is stable for the process.
- `queries.snapshot` right after Lens answers `welcome`, and again on
  `snapshot.request`.
- `query.event` and `mutation.event` for every cache change, following the
  delta policy described in [`protocol/README.md`](../../protocol/README.md);
  a field the device cleared (`data: undefined` after `TRIGGER_LOADING`,
  `TRIGGER_ERROR`, `RESET`) travels as an explicit `null`.
- `online.status` when `onlineManager` changes; `online.set` from Lens calls
  `onlineManager.setOnline`.
- `action.result` for every `query.action` Lens sends. The actions run the
  recipes of the official TanStack Query devtools; in particular `SET_QUERY_DATA`
  calls `queryClient.setQueryData(queryKey, data)`, so observers re-render and
  `dataUpdatedAt` and `dataUpdateCount` advance as after a fetch. An action
  without `data` is answered with `ok: false`.

Payloads go through a safe serializer: BigInt becomes a string, Date an ISO
string, functions `"[Function]"`, circular references `"[Circular]"`, Map and
Set arrays, Error `{ name, message, stack }`. Values above 1 MiB are replaced
with `{ "__lensTruncated": true, "bytes": n }` and the query or mutation gets
`dataTruncated: true`.

A whole `queries.snapshot` is capped at `SNAPSHOT_MAX_BYTES` (6 MiB, exported):
when the serialized snapshot would exceed it, the largest values among query
`data` and mutation `variables`, `data` and `context` are replaced with the same
marker, largest first, until the frame fits. Every query and mutation stays
listed; the client warns through `logger` how many were cut. Lens reads frames
of at most `SNAPSHOT_FRAME_MAX_BYTES` (8 MiB, exported); a snapshot still above
that after capping (query keys, `meta` and errors are not capped) is not sent,
because Lens would close the socket with 1009 and the reconnect would resend
it. The client logs one warning with the size and the limit, keeps sending
cache events, and tries again on the next `snapshot.request` or reconnect.

## Production behavior

`enabled` defaults to `__DEV__` under React Native. In a production bundle the
client is a no-op: `connect`, `send` and `registerPlugin` do nothing and the
state is `"disabled"`. The package has `"sideEffects": false`, so bundlers can
drop what you do not import.

## Troubleshooting

- **Android emulator cannot connect.** The emulator reaches the host machine
  as `10.0.2.2`; the SDK maps `localhost` there automatically. If Metro runs on
  another machine, pass `url: "ws://<that-ip>:7420/lens"` or run
  `adb reverse tcp:7420 tcp:7420` and keep `localhost`.
- **Physical device on the LAN.** The SDK uses the host of the Metro bundle
  URL, so the device connects to the machine that serves the bundle. Make sure
  port 7420 is reachable (firewall) and that Lens listens on `0.0.0.0`, which is
  the default.
- **Lens closes the socket right away.** Lens was started with `--sdk-token`;
  pass the same value as `token`. A protocol mismatch closes with code 4002:
  update either side. Both codes (4003 token, 4002 protocol) are fatal: the
  client logs a warning naming the code, goes `disabled` and stops
  reconnecting until `connect()` is called again. Code 4001 (no `hello` within
  the handshake timeout) is fatal after three consecutive closes without a
  `welcome` in between; a `welcome` resets the count. Every other close keeps
  reconnecting with backoff.
- **Nothing shows up.** Pass `logger: console` to see the resolved URL,
  reconnect attempts and the Lens version after `welcome`. Check that
  `queryClient` is the instance your `QueryClientProvider` uses.
- **Expo Go.** Works out of the box in development; the SDK only needs the
  global `WebSocket`.

## Plugins (reserved)

`client.registerPlugin({ id, onMessage })` observes every inbound frame and is
reserved for future Lens features; use `client.send` to emit frames.
