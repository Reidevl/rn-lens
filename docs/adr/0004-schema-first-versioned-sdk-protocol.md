# 0004. Schema-first, versioned SDK protocol with contract tests

Date: 2026-11-17
Status: Accepted

## Context

The SDK channel (ADR-0002) is a public contract between a Go server and a
TypeScript package that third-party plugins will also speak. The message set is
small (around fifteen messages) but union-heavy: every message is tagged by a
`type` field. Go code generators for JSON Schema handle tagged unions poorly, and
TypeScript generators handle them well.

## Decision

- The source of truth is hand-written **JSON Schema 2020-12** under `protocol/`,
  one schema per message plus an envelope, with example fixtures under
  `protocol/fixtures`.
- TypeScript types are generated from the schema (`json-schema-to-typescript`).
- Go structs are written by hand and proven by a conformance test that validates
  every fixture against the schema and round-trips it through the Go types.
  Vitest runs the same fixtures against the generated TypeScript with Ajv.
- Every message carries `protocolVersion`. The server rejects an unknown major
  with a typed error the UI displays; minors are additive only.

## Consequences

- Both sides can change independently as long as the fixtures pass.
- Plugin authors get a readable schema and real example payloads.
- Breaking changes are explicit: a new major, a new fixture set, and a migration
  note in the docs.
