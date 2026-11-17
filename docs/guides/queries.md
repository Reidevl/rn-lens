# Queries and Mutations

The Queries and Mutations panels show the TanStack Query caches of your app
and let you act on them the way the official devtools do, from Lens's window
instead of an overlay inside the app.

## Quick path

1. `npm install @rnlens/sdk`
2. Mount the hook once, next to your `QueryClientProvider`:

   ```tsx
   import { useLens } from "@rnlens/sdk";

   export function App() {
     useLens({ queryClient });
     return <QueryClientProvider client={queryClient}>…</QueryClientProvider>;
   }
   ```

3. Start the app in development. It appears in **Queries** within a second.

The SDK is a no-op in production bundles, so the hook can stay in place.

## What you see

| Column or control | Meaning                                                                                                                                                      |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Status dot        | `fresh`, `fetching`, `paused`, `stale`, `inactive`, computed with the official devtools rule (fetching wins, then no observers, then paused, then staleness) |
| Observers         | How many components use the query right now                                                                                                                  |
| Updated           | When data last arrived                                                                                                                                       |
| Status chips      | Filter by status; each shows its count                                                                                                                       |
| Online / Offline  | Flips `onlineManager` on the device, so you can watch paused fetches and retries                                                                             |

Selecting a query opens its key, timestamps, data as a tree and the actions:

| Action                            | What runs in the app                                                                   |
| --------------------------------- | -------------------------------------------------------------------------------------- |
| Refetch                           | `query.fetch()`                                                                        |
| Invalidate                        | `queryClient.invalidateQueries({ queryKey, exact: true })`                             |
| Reset                             | `queryClient.resetQueries(...)`                                                        |
| Remove                            | `queryClient.removeQueries(...)`                                                       |
| Trigger loading / Restore loading | A never-resolving `queryFn` is swapped in and the previous options are kept to restore |
| Trigger error / Restore error     | The state is set to `error`; restore resets the query                                  |
| Edit data                         | `query.setState({ data })` with the JSON you typed                                     |
| Clear cache                       | `queryCache.clear()` or `mutationCache.clear()`                                        |

Mutations list every mutation with its status, variables, data and error.

## How it connects

The SDK opens a WebSocket to Lens (`ws://<host>:7420/lens`) and announces the
app, device and protocol version. The host is taken from the Metro bundle
URL, so simulators, Android emulators (`10.0.2.2`) and physical devices on the
same network work without configuration. Lens keeps one session per app and
device; a reconnect replaces the previous one.

The wire format is documented in [`protocol/README.md`](../../protocol/README.md).
Only cache changes travel: after an initial snapshot the SDK sends per-action
deltas. Payloads larger than 1 MiB are replaced by a marker, by the SDK before
sending and again by Lens on receipt (the panel says "data truncated").
The snapshot as a whole is capped at 6 MiB: above that, the largest queries
arrive with their data truncated so the snapshot still fits in one frame.

## Options

| Option                  | Default            | Use                                                 |
| ----------------------- | ------------------ | --------------------------------------------------- |
| `queryClient`           | none               | Required for the Queries and Mutations panels       |
| `url`                   | derived from Metro | Point at another host or port (`rnlens --sdk-port`) |
| `token`                 | none               | Pairing token when Lens runs with `--sdk-token`     |
| `deviceName`, `appName` | detected           | Override what the device picker shows               |
| `enabled`               | `__DEV__`          | Force on or off                                     |

## Troubleshooting

- **No app connected.** Check that the app runs a development build, that it loaded the bundle from Metro (the host comes from there), and that port 7420 is reachable from the device. `rnlens --sdk-port` changes the port.
- **Physical device on another network.** Pass `url: "ws://<your computer's IP>:7420/lens"`.
- **Token mismatch.** The socket closes with code 4003; pass the same `token` to `useLens`. The SDK logs a warning and stops reconnecting until the app reloads or calls `connect()` again.
- **Older protocol.** Code 4002 means the SDK and Lens disagree on the protocol major; update both. Like 4003, it is fatal for the SDK: no reconnect until `connect()` runs again.
