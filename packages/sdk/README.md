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
| `url`           | `string`               | derived, see below                          | Full WebSocket URL of Lens.                                            |
| `token`         | `string`               | —                                           | Pairing token when Lens runs with `--sdk-token`.                       |
| `deviceName`    | `string`               | device model                                | Name shown in the Lens device list.                                    |
| `appName`       | `string`               | `appId`                                     | App name shown in Lens.                                                |
| `appId`         | `string`               | `appName` or `"app"`                        | Bundle id or package name; sessions are keyed by app id and device id. |
| `enabled`       | `boolean`              | `__DEV__`, else `NODE_ENV !== "production"` | When false every method is a no-op and no socket is opened.            |
| `logger`        | `LensLogger`           | silent                                      | Pass `console` to see connection diagnostics.                          |
| `WebSocketImpl` | `WebSocketConstructor` | global `WebSocket`                          | Alternative WebSocket implementation (tests, custom runtimes).         |
| `backoff`       | `{ minMs?, maxMs? }`   | `{ minMs: 500, maxMs: 10000 }`              | Reconnect backoff bounds.                                              |

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
  the Metro script URL and the feature list (`["queries"]` when a
  `queryClient` is given). The device id is persisted with
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
