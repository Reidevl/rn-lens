/* eslint-disable */
/**
 * Generated from protocol/schema/lens-protocol.v1.schema.json by `pnpm --filter @rnlens/protocol run generate`.
 * Do not edit by hand.
 */

/**
 * Every frame exchanged between @rnlens/sdk and Lens over the SDK WebSocket. Protocol version 1.
 */
export type LensMessage =
  | HelloMessage
  | QueriesSnapshotMessage
  | QueryEventMessage
  | MutationEventMessage
  | OnlineStatusMessage
  | ActionResultMessage
  | PongMessage
  | WelcomeMessage
  | SnapshotRequestMessage
  | QueryActionMessage
  | OnlineSetMessage
  | PingMessage
  | StorageSnapshotMessage
  | StorageEventMessage
  | StorageActionMessage
  | StorageResultMessage;
/**
 * Any JSON value (object, array, string, number, boolean or null).
 */
export type JsonValue = {} | unknown[] | string | number | boolean | null;
/**
 * TanStack Query key: an array of JSON values.
 */
export type QueryKey = JsonValue[];
export type QueryStatus = "pending" | "error" | "success";
export type FetchStatus = "fetching" | "paused" | "idle";
export type MutationStatus = "idle" | "pending" | "success" | "error";
export type QueryEventKind = "added" | "updated" | "removed" | "observers";
export type MutationEventKind = "added" | "updated" | "removed";
export type QueryActionType =
  | "REFETCH"
  | "INVALIDATE"
  | "RESET"
  | "REMOVE"
  | "SET_QUERY_DATA"
  | "TRIGGER_ERROR"
  | "RESTORE_ERROR"
  | "TRIGGER_LOADING"
  | "RESTORE_LOADING"
  | "CLEAR_QUERY_CACHE"
  | "CLEAR_MUTATION_CACHE";
export type StorageKind = "async-storage" | "mmkv" | "secure-store";
export type StorageActionType = "GET" | "SET" | "REMOVE" | "CLEAR" | "REFRESH";

/**
 * First message the SDK sends after the socket opens.
 */
export interface HelloMessage {
  v: 1;
  type: "hello";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: HelloPayload;
}
export interface HelloPayload {
  sdkVersion: string;
  protocolVersion: number;
  app: AppInfo;
  device: DeviceInfo;
  /**
   * Metro bundle URL the app loaded, used for diagnostics.
   */
  scriptURL?: string;
  features: ("queries" | "storage" | "plugins")[];
  /**
   * Pairing token when Lens runs with --sdk-token.
   */
  token?: string;
}
export interface AppInfo {
  /**
   * Bundle id or package name.
   */
  id: string;
  name?: string;
  platform: "ios" | "android" | "web" | "macos" | "windows" | "other";
  rnVersion?: string;
}
export interface DeviceInfo {
  /**
   * Stable per install; the SDK persists it.
   */
  id: string;
  name?: string;
}
/**
 * Full cache contents; sent after welcome and on snapshot.request.
 */
export interface QueriesSnapshotMessage {
  v: 1;
  type: "queries.snapshot";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: QueriesSnapshotPayload;
}
export interface QueriesSnapshotPayload {
  queries: Query[];
  mutations: Mutation[];
  online: boolean;
}
export interface Query {
  /**
   * Stable hash of the key, as TanStack computes it.
   */
  queryHash: string;
  queryKey: QueryKey;
  state: QueryState;
  observersCount: number;
  /**
   * At least one enabled observer.
   */
  isActive: boolean;
  /**
   * As computed on the device with the observers' staleTime.
   */
  isStale: boolean;
  /**
   * Milliseconds; Infinity is sent as -1.
   */
  gcTime?: number;
  meta?: JsonValue;
  /**
   * data was cut at the SDK's size cap.
   */
  dataTruncated?: boolean;
}
export interface QueryState {
  status: QueryStatus;
  fetchStatus: FetchStatus;
  /**
   * Milliseconds since epoch; 0 when never updated.
   */
  dataUpdatedAt: number;
  errorUpdatedAt: number;
  dataUpdateCount: number;
  errorUpdateCount: number;
  fetchFailureCount: number;
  isInvalidated: boolean;
  data?: JsonValue;
  error?: JsonValue;
  fetchMeta?: JsonValue;
}
export interface Mutation {
  mutationId: number;
  mutationKey?: QueryKey;
  state: MutationState;
  scope?: JsonValue;
  meta?: JsonValue;
  dataTruncated?: boolean;
}
export interface MutationState {
  status: MutationStatus;
  submittedAt: number;
  isPaused: boolean;
  variables?: JsonValue;
  data?: JsonValue;
  error?: JsonValue;
  failureCount?: number;
  failureReason?: JsonValue;
  context?: JsonValue;
}
/**
 * One query cache change.
 */
export interface QueryEventMessage {
  v: 1;
  type: "query.event";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: QueryEventPayload;
}
/**
 * added: query present. updated: delta present (or query when the action is unknown). removed: hash only. observers: observersCount/isActive/isStale present.
 */
export interface QueryEventPayload {
  kind: QueryEventKind;
  queryHash: string;
  query?: Query;
  /**
   * TanStack action type for kind=updated (fetch, success, error, setState, invalidate, pause, continue, ...).
   */
  action?: string;
  delta?: QueryStateDelta;
  observersCount?: number;
  isActive?: boolean;
  isStale?: boolean;
}
/**
 * Subset of QueryState fields that changed.
 */
export interface QueryStateDelta {
  status?: QueryStatus;
  fetchStatus?: FetchStatus;
  /**
   * Milliseconds since epoch; 0 when never updated.
   */
  dataUpdatedAt?: number;
  errorUpdatedAt?: number;
  dataUpdateCount?: number;
  errorUpdateCount?: number;
  fetchFailureCount?: number;
  isInvalidated?: boolean;
  data?: JsonValue;
  error?: JsonValue;
  fetchMeta?: JsonValue;
}
/**
 * One mutation cache change.
 */
export interface MutationEventMessage {
  v: 1;
  type: "mutation.event";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: MutationEventPayload;
}
export interface MutationEventPayload {
  kind: MutationEventKind;
  mutationId: number;
  mutation?: Mutation;
  delta?: MutationStateDelta;
}
export interface MutationStateDelta {
  status?: MutationStatus;
  submittedAt?: number;
  isPaused?: boolean;
  variables?: JsonValue;
  data?: JsonValue;
  error?: JsonValue;
  failureCount?: number;
  failureReason?: JsonValue;
  context?: JsonValue;
}
/**
 * onlineManager changed on the device.
 */
export interface OnlineStatusMessage {
  v: 1;
  type: "online.status";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: OnlineStatusPayload;
}
export interface OnlineStatusPayload {
  online: boolean;
}
/**
 * Outcome of a query.action.
 */
export interface ActionResultMessage {
  v: 1;
  type: "action.result";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: ActionResultPayload;
}
export interface ActionResultPayload {
  /**
   * The query.action id this answers.
   */
  id: string;
  ok: boolean;
  error?: string;
}
/**
 * Reply to ping.
 */
export interface PongMessage {
  v: 1;
  type: "pong";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: EmptyPayload;
}
export interface EmptyPayload {}
/**
 * Lens accepted the hello.
 */
export interface WelcomeMessage {
  v: 1;
  type: "welcome";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: WelcomePayload;
}
export interface WelcomePayload {
  lensVersion: string;
  protocolVersion: number;
  sessionId: string;
}
/**
 * Lens asks for a fresh queries.snapshot.
 */
export interface SnapshotRequestMessage {
  v: 1;
  type: "snapshot.request";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: EmptyPayload;
}
/**
 * Lens asks the device to run a devtools action.
 */
export interface QueryActionMessage {
  v: 1;
  type: "query.action";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: QueryActionPayload;
}
export interface QueryActionPayload {
  /**
   * Correlates with action.result.
   */
  id: string;
  type: QueryActionType;
  /**
   * Required for every type except the CLEAR_* ones.
   */
  queryHash?: string;
  data?: JsonValue;
}
/**
 * Lens sets onlineManager on the device.
 */
export interface OnlineSetMessage {
  v: 1;
  type: "online.set";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: OnlineSetPayload;
}
export interface OnlineSetPayload {
  online: boolean;
}
/**
 * Liveness probe; the SDK answers with pong.
 */
export interface PingMessage {
  v: 1;
  type: "ping";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: EmptyPayload;
}
/**
 * The app's storage stores and their keys (values are fetched on demand).
 */
export interface StorageSnapshotMessage {
  v: 1;
  type: "storage.snapshot";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: StorageSnapshotPayload;
}
export interface StorageSnapshotPayload {
  stores: StorageStore[];
  /**
   * Set when the snapshot replaces only this store.
   */
  storeId?: string;
}
export interface StorageStore {
  /**
   * Stable id the SDK chose, for example "async-storage" or "mmkv:default".
   */
  id: string;
  kind: StorageKind;
  name: string;
  keys: StorageKey[];
  /**
   * The store has more keys than the snapshot carries (10,000).
   */
  truncated?: boolean;
  /**
   * The SDK reports changes as they happen (MMKV); otherwise Lens refreshes.
   */
  live?: boolean;
  /**
   * Only the keys the app declared are listed (SecureStore cannot enumerate).
   */
  declaredKeys?: boolean;
}
export interface StorageKey {
  key: string;
  /**
   * Bytes of the value, when cheap to know.
   */
  size?: number;
}
/**
 * A key changed in a live store (MMKV).
 */
export interface StorageEventMessage {
  v: 1;
  type: "storage.event";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: StorageEventPayload;
}
export interface StorageEventPayload {
  storeId: string;
  kind: "set" | "removed" | "cleared";
  key?: string;
  size?: number;
}
/**
 * Lens asks the SDK to read or change a store.
 */
export interface StorageActionMessage {
  v: 1;
  type: "storage.action";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: StorageActionPayload;
}
export interface StorageActionPayload {
  /**
   * Correlates with storage.result.
   */
  id: string;
  type: StorageActionType;
  storeId: string;
  key?: string;
  value?: StorageValue;
}
/**
 * A stored value. Bytes travel as base64 in value.
 */
export interface StorageValue {
  type: "string" | "number" | "boolean" | "bytes";
  /**
   * The value as text: numbers and booleans in JSON form, bytes as base64.
   */
  value: string;
  /**
   * The value was larger than the SDK sends (1 MiB); value is empty.
   */
  truncated?: boolean;
  /**
   * Size in bytes of the full value.
   */
  size?: number;
}
/**
 * Outcome of a storage.action.
 */
export interface StorageResultMessage {
  v: 1;
  type: "storage.result";
  /**
   * Optional correlation id.
   */
  id?: string;
  payload: StorageResultPayload;
}
export interface StorageResultPayload {
  /**
   * The storage.action id this answers.
   */
  id: string;
  ok: boolean;
  error?: string;
  value?: StorageValue1;
}
/**
 * A stored value. Bytes travel as base64 in value.
 */
export interface StorageValue1 {
  type: "string" | "number" | "boolean" | "bytes";
  /**
   * The value as text: numbers and booleans in JSON form, bytes as base64.
   */
  value: string;
  /**
   * The value was larger than the SDK sends (1 MiB); value is empty.
   */
  truncated?: boolean;
  /**
   * Size in bytes of the full value.
   */
  size?: number;
}
