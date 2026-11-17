# Lens SDK protocol

The JSON Schema in `schema/` is the source of truth for every frame exchanged
between `@rnlens/sdk` (inside the app) and Lens over the SDK WebSocket
(`ws://<host>:7420/lens`). Fixtures in `fixtures/` are real example frames;
both the Go server and the TypeScript SDK prove themselves against them.

## Envelope

```json
{ "v": 1, "type": "query.event", "id": "optional correlation id", "payload": {} }
```

`v` is the protocol major. A server that does not speak the major closes the
socket with code 4002. Minors are additive: new optional fields and new message
types only.

## Messages

| Direction               | `type`             | Purpose                                                                                  |
| ----------------------- | ------------------ | ---------------------------------------------------------------------------------------- |
| SDK → Lens              | `hello`            | identity, app, device, features; must be the first frame within 5 s                      |
| Lens → SDK              | `welcome`          | accepted; carries the Lens version and a session id                                      |
| Lens → SDK              | `snapshot.request` | ask for a full `queries.snapshot`                                                        |
| SDK → Lens              | `queries.snapshot` | every query and mutation plus the online flag                                            |
| SDK → Lens              | `query.event`      | one cache change: `added`, `updated` (with `action` and `delta`), `removed`, `observers` |
| SDK → Lens              | `mutation.event`   | one mutation cache change                                                                |
| SDK → Lens              | `online.status`    | `onlineManager` changed on the device                                                    |
| Lens → SDK              | `query.action`     | run a devtools action; answered by `action.result` with the same `id`                    |
| Lens → SDK              | `online.set`       | set `onlineManager` on the device                                                        |
| Lens → SDK / SDK → Lens | `ping` / `pong`    | liveness                                                                                 |

## Delta policy for `query.event` (`kind: updated`)

The SDK sends only the fields a cache action changes. The mapping follows the
MIT-licensed `@rozenite/tanstack-query-plugin` by Callstack:

| TanStack action     | Fields in `delta`                                                           |
| ------------------- | --------------------------------------------------------------------------- |
| `fetch`             | `status`, `fetchStatus`, `fetchMeta`, `dataUpdatedAt`, `errorUpdatedAt`     |
| `success`           | `status`, `data`, `dataUpdatedAt`, `error`, `errorUpdatedAt`, `fetchStatus` |
| `error`             | `status`, `error`, `errorUpdatedAt`, `fetchStatus`                          |
| `setState`          | the state passed to `setState`                                              |
| `invalidate`        | `isInvalidated`                                                             |
| `pause`, `continue` | `fetchStatus`                                                               |
| anything else       | the full `query` instead of a `delta`                                       |

## Payload limits

`data`, `error`, `variables` and `context` are serialized with a safe
serializer (BigInt as string, Date as ISO string, circular references as
`"[Circular]"`, depth 20) and cut at 1 MiB; a cut value sets `dataTruncated`.

## Regenerating types

```bash
pnpm --filter @rnlens/protocol run generate   # writes src/types.ts
pnpm --filter @rnlens/protocol test           # validates every fixture with Ajv
go test ./internal/protocol/...               # validates fixtures and strict-decodes them in Go
```
