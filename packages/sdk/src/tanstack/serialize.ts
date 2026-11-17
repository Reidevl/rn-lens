import type {
  JsonValue,
  Mutation as ProtoMutation,
  MutationState as ProtoMutationState,
  MutationStateDelta,
  QueriesSnapshotMessage,
  Query as ProtoQuery,
  QueryKey as ProtoQueryKey,
  QueryState as ProtoQueryState,
  QueryStateDelta,
} from "@rnlens/protocol";
import type { Mutation, Query, QueryState } from "@tanstack/react-query";

import { jsonByteLength, safeSerialize, toSafeJson, type TruncationMarker } from "../serialize";

export interface SerializeLimits {
  maxBytes: number;
  maxDepth: number;
}

/**
 * Budget for a whole `queries.snapshot` frame. Lens reads frames of at most
 * 8 MiB; a snapshot above this budget has the `data` of its largest queries
 * replaced by the truncation marker until it fits, every query stays listed.
 */
export const SNAPSHOT_MAX_BYTES = 6 * 1024 * 1024;

/**
 * The generated `JsonValue` type is an object index signature, while the schema
 * accepts any JSON value; this cast is the single place where the two meet.
 */
export const asJson = (value: unknown): JsonValue => value as JsonValue;

type QueryDeltaKey = keyof QueryStateDelta;

export const QUERY_DELTA_KEYS: readonly QueryDeltaKey[] = [
  "status",
  "fetchStatus",
  "dataUpdatedAt",
  "errorUpdatedAt",
  "dataUpdateCount",
  "errorUpdateCount",
  "fetchFailureCount",
  "isInvalidated",
  "data",
  "error",
  "fetchMeta",
];

const SERIALIZED_QUERY_KEYS: ReadonlySet<QueryDeltaKey> = new Set(["data", "error", "fetchMeta"]);

/** Which query state fields each TanStack action changes (see protocol/README.md). */
export const QUERY_ACTION_DELTA_FIELDS: Readonly<Record<string, readonly QueryDeltaKey[]>> = {
  fetch: ["status", "fetchStatus", "fetchMeta", "dataUpdatedAt", "errorUpdatedAt"],
  success: ["status", "data", "dataUpdatedAt", "error", "errorUpdatedAt", "fetchStatus"],
  error: ["status", "error", "errorUpdatedAt", "fetchStatus"],
  invalidate: ["isInvalidated"],
  pause: ["fetchStatus"],
  continue: ["fetchStatus"],
};

function serializeQueryKey(key: readonly unknown[], limits: SerializeLimits): ProtoQueryKey {
  const safe = toSafeJson(Array.from(key), limits.maxDepth);
  return (Array.isArray(safe) ? safe : [safe]) as ProtoQueryKey;
}

/**
 * Copies the listed fields from a (partial) TanStack query state into a delta,
 * serializing `data`, `error` and `fetchMeta` safely. Returns whether `data`
 * was truncated.
 */
export function pickQueryState(
  state: Partial<QueryState>,
  keys: readonly QueryDeltaKey[],
  limits: SerializeLimits,
): { delta: QueryStateDelta; truncated: boolean } {
  const delta: Record<string, unknown> = {};
  let truncated = false;
  for (const key of keys) {
    if (!(key in state)) continue;
    const raw = state[key as keyof QueryState];
    if (raw === undefined) continue;
    if (SERIALIZED_QUERY_KEYS.has(key)) {
      const result = safeSerialize(raw, limits);
      if (result.value === undefined) continue;
      delta[key] = result.value;
      if (result.truncated) truncated = true;
    } else {
      delta[key] = raw;
    }
  }
  return { delta: delta as QueryStateDelta, truncated };
}

/** Full wire representation of a TanStack query. */
export function serializeQuery(query: Query, limits: SerializeLimits): ProtoQuery {
  const { delta, truncated } = pickQueryState(query.state, QUERY_DELTA_KEYS, limits);
  const state = delta as ProtoQueryState;
  const out: ProtoQuery = {
    queryHash: query.queryHash,
    queryKey: serializeQueryKey(query.queryKey, limits),
    state,
    observersCount: query.getObserversCount(),
    isActive: query.isActive(),
    isStale: query.isStale(),
    gcTime: query.gcTime === Infinity ? -1 : query.gcTime,
  };
  if (query.meta !== undefined) out.meta = asJson(toSafeJson(query.meta, limits.maxDepth));
  if (truncated) out.dataTruncated = true;
  return out;
}

const MUTATION_STATE_KEYS = [
  "status",
  "submittedAt",
  "isPaused",
  "variables",
  "data",
  "error",
  "failureCount",
  "failureReason",
  "context",
] as const;

const SERIALIZED_MUTATION_KEYS: ReadonlySet<string> = new Set([
  "variables",
  "data",
  "error",
  "failureReason",
  "context",
]);

/** Wire representation of a mutation state; also used as the `updated` delta. */
export function serializeMutationState(
  mutation: Mutation,
  limits: SerializeLimits,
): { state: ProtoMutationState; truncated: boolean } {
  const state: Record<string, unknown> = {};
  let truncated = false;
  for (const key of MUTATION_STATE_KEYS) {
    const raw = mutation.state[key];
    if (raw === undefined) continue;
    if (SERIALIZED_MUTATION_KEYS.has(key)) {
      const result = safeSerialize(raw, limits);
      if (result.value === undefined) continue;
      state[key] = result.value;
      if (result.truncated && key === "data") truncated = true;
    } else {
      state[key] = raw;
    }
  }
  return { state: state as unknown as ProtoMutationState, truncated };
}

/** Full wire representation of a TanStack mutation. */
export function serializeMutation(mutation: Mutation, limits: SerializeLimits): ProtoMutation {
  const { state, truncated } = serializeMutationState(mutation, limits);
  const out: ProtoMutation = { mutationId: mutation.mutationId, state };
  if (mutation.options.mutationKey !== undefined) {
    out.mutationKey = serializeQueryKey(mutation.options.mutationKey, limits);
  }
  if (mutation.options.scope !== undefined) {
    out.scope = asJson(toSafeJson(mutation.options.scope, limits.maxDepth));
  }
  if (mutation.meta !== undefined) out.meta = asJson(toSafeJson(mutation.meta, limits.maxDepth));
  if (truncated) out.dataTruncated = true;
  return out;
}

export function mutationDelta(mutation: Mutation, limits: SerializeLimits): MutationStateDelta {
  return serializeMutationState(mutation, limits).state;
}

export interface CapSnapshotResult {
  /** Serialized frame, at most `maxBytes` when enough query data could be dropped. */
  json: string;
  bytes: number;
  /** Hashes whose `data` became the truncation marker to fit the budget. */
  truncated: string[];
}

/**
 * Serializes a `queries.snapshot` frame within `maxBytes`: while the frame is
 * too large, the query holding the most data bytes has its `data` replaced by
 * `{ "__lensTruncated": true, "bytes": n }` and gets `dataTruncated: true`.
 * Queries are never dropped. Mutates the queries of `message` in place.
 */
export function capSnapshot(
  message: QueriesSnapshotMessage,
  maxBytes: number = SNAPSHOT_MAX_BYTES,
): CapSnapshotResult {
  let json = JSON.stringify(message);
  let bytes = jsonByteLength(json);
  const truncated: string[] = [];
  if (bytes <= maxBytes) return { json, bytes, truncated };

  const candidates = message.payload.queries
    .filter((query) => query.state.data !== undefined && query.dataTruncated !== true)
    .map((query) => ({ query, dataBytes: jsonByteLength(JSON.stringify(query.state.data)) }))
    .sort((a, b) => b.dataBytes - a.dataBytes);
  for (const { query, dataBytes } of candidates) {
    if (bytes <= maxBytes) break;
    const marker: TruncationMarker = { __lensTruncated: true, bytes: dataBytes };
    query.state.data = asJson(marker);
    query.dataTruncated = true;
    truncated.push(query.queryHash);
    bytes -= dataBytes - jsonByteLength(JSON.stringify(marker));
  }
  json = JSON.stringify(message);
  return { json, bytes: jsonByteLength(json), truncated };
}
