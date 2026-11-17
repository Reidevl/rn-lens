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
socket with code 4002.

## Compatibility rule

Minors are additive: a minor may add optional fields and new message types,
never remove or retype a field. Removing or retyping a field (or making an
optional field required) is a major bump, which changes `v`.

Decoders therefore **ignore fields they do not know**, in the envelope and in
every payload: Lens (`internal/protocol`) decodes with unknown fields allowed,
and `@rnlens/sdk` checks only the envelope shape, so an SDK built against a
newer minor talks to an older Lens and the other way around. Lens ignores a
message `type` it does not know; the SDK logs it at debug level.

The JSON Schema is exact: it describes precisely what one minor defines
(`additionalProperties: false` everywhere), and the fixtures under `fixtures/`
are validated strictly on both sides (`go test ./internal/protocol/...` uses
`DecodeStrict` / `PayloadIntoStrict`). A field that reaches a fixture without
reaching the schema, or the schema without reaching the Go types, fails the
build. Strictness lives in the conformance tests, tolerance in the decoders.

## Close codes

| Code   | Sent when                                                                               |
| ------ | --------------------------------------------------------------------------------------- |
| `4001` | no `hello` within the handshake timeout                                                 |
| `4002` | the envelope `v` or `hello.protocolVersion` is another major (also after the handshake) |
| `4003` | pairing token missing or wrong                                                          |
| `4004` | another connection for the same app and device replaced this one                        |
| `4005` | no `pong` after the allowed missed pings                                                |
| `4006` | protocol error: first frame not `hello`, undecodable JSON, non-text frame               |
| `1001` | Lens is shutting down                                                                   |

After the handshake an undecodable or non-text frame closes the session with
`4006` and an envelope with another major closes it with `4002`. The SDK treats
`4002` and `4003` as fatal: it warns, goes `disabled` and stops reconnecting;
every other close is followed by a reconnect with backoff.

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

Lens reads frames of at most 8 MiB. A `queries.snapshot` is capped as a whole
at 6 MiB (`SNAPSHOT_MAX_BYTES` in `@rnlens/sdk`): above that, the SDK replaces
the `data` of the largest queries with the truncation marker, largest first,
until the frame fits, keeping every query listed.

## Regenerating types

```bash
pnpm --filter @rnlens/protocol run generate   # writes src/types.ts
pnpm --filter @rnlens/protocol test           # validates every fixture with Ajv
go test ./internal/protocol/...               # validates fixtures and strict-decodes them in Go
```
