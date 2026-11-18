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
 * Budget for a whole `queries.snapshot` frame. A snapshot above this budget
 * has the largest `data` of its queries and `variables`, `data` and `context`
 * of its mutations replaced by the truncation marker until it fits; every
 * query and mutation stays listed.
 */
export const SNAPSHOT_MAX_BYTES = 6 * 1024 * 1024;

/**
 * Largest frame Lens reads (`internal/adapters/sdkws`, 8 MiB). A snapshot
 * still above it after capping is not sent: Lens would close the socket with
 * 1009 and the reconnect would resend the same frame.
 */
export const SNAPSHOT_FRAME_MAX_BYTES = 8 * 1024 * 1024;

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

export interface PickOptions {
  /**
   * In a delta, a listed key whose value is `undefined` on the device (data or
   * error cleared by TRIGGER_LOADING, TRIGGER_ERROR, RESET) is sent as JSON
   * `null` so Lens clears it too; absent means unchanged. A full query keeps
   * omitting it.
   */
  explicitNull?: boolean;
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
  options: PickOptions = {},
): { delta: QueryStateDelta; truncated: boolean } {
  const delta: Record<string, unknown> = {};
  let truncated = false;
  for (const key of keys) {
    if (!(key in state)) continue;
    const raw = state[key as keyof QueryState];
    if (raw === undefined) {
      if (options.explicitNull) delta[key] = null;
      continue;
    }
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
  /** Serialized frame, at most `maxBytes` when enough payload could be dropped. */
  json: string;
  bytes: number;
  /** Hashes whose `data` became the truncation marker to fit the budget. */
  truncated: string[];
  /** Ids of mutations whose `variables`, `data` or `context` became the marker. */
  truncatedMutations: number[];
}

/** Mutation state fields the snapshot cap may replace with the marker. */
const CAPPED_MUTATION_KEYS = ["variables", "data", "context"] as const;

/** One value the cap may replace, with the bytes its replacement saves. */
interface CapCandidate {
  bytes: number;
  cut(): void;
}

function isTruncationMarker(value: unknown): boolean {
  return (
    typeof value === "object" &&
    value !== null &&
    (value as { __lensTruncated?: unknown }).__lensTruncated === true
  );
}

function truncationMarker(bytes: number): JsonValue {
  const marker: TruncationMarker = { __lensTruncated: true, bytes };
  return asJson(marker);
}

/**
 * Serializes a `queries.snapshot` frame within `maxBytes`: while the frame is
 * too large, the largest value among query `data` and mutation `variables`,
 * `data` and `context` is replaced by `{ "__lensTruncated": true, "bytes": n }`;
 * a cut `data` also sets `dataTruncated: true`. Queries and mutations are
 * never dropped. Mutates `message` in place.
 */
export function capSnapshot(
  message: QueriesSnapshotMessage,
  maxBytes: number = SNAPSHOT_MAX_BYTES,
): CapSnapshotResult {
  let json = JSON.stringify(message);
  let bytes = jsonByteLength(json);
  const truncated: string[] = [];
  const truncatedMutations = new Set<number>();
  if (bytes <= maxBytes) return { json, bytes, truncated, truncatedMutations: [] };

  const candidates: CapCandidate[] = [];
  for (const query of message.payload.queries) {
    if (query.state.data === undefined || query.dataTruncated === true) continue;
    const dataBytes = jsonByteLength(JSON.stringify(query.state.data));
    candidates.push({
      bytes: dataBytes,
      cut: () => {
        query.state.data = truncationMarker(dataBytes);
        query.dataTruncated = true;
        truncated.push(query.queryHash);
      },
    });
  }
  for (const mutation of message.payload.mutations) {
    const state = mutation.state as unknown as Record<string, unknown>;
    for (const key of CAPPED_MUTATION_KEYS) {
      const value = state[key];
      if (value === undefined || isTruncationMarker(value)) continue;
      const valueBytes = jsonByteLength(JSON.stringify(value));
      candidates.push({
        bytes: valueBytes,
        cut: () => {
          state[key] = truncationMarker(valueBytes);
          if (key === "data") mutation.dataTruncated = true;
          truncatedMutations.add(mutation.mutationId);
        },
      });
    }
  }
  candidates.sort((a, b) => b.bytes - a.bytes);
  for (const candidate of candidates) {
    if (bytes <= maxBytes) break;
    candidate.cut();
    bytes -= candidate.bytes - jsonByteLength(JSON.stringify(truncationMarker(candidate.bytes)));
  }
  json = JSON.stringify(message);
  return {
    json,
    bytes: jsonByteLength(json),
    truncated,
    truncatedMutations: [...truncatedMutations],
  };
}
