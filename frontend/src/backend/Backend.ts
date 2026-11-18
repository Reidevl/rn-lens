/**
 * The Backend is the only door between the UI and the Go core.
 *
 * Features never import Wails directly; they receive a Backend through
 * `useBackend()`. This keeps panels testable with a mock and lets a WebSocket
 * implementation (browser / headless mode) ship later without touching them.
 * Streams follow notify-then-pull (ADR-0003): the backend emits small change
 * notifications and the UI pulls the visible range on demand.
 */

export interface AppInfo {
  name: string;
  version: string;
}

export interface Capabilities {
  nativePageReloads: boolean;
  nativeSourceCodeFetching: boolean;
  supportsMultipleDebuggers: boolean;
}

export interface Target {
  id: string;
  endpoint: string;
  metroId: string;
  pageId: string;
  title: string;
  appId: string;
  deviceName: string;
  logicalDeviceId: string;
  webSocketUrl: string;
  vm?: string;
  capabilities: Capabilities;
  reloadable: boolean;
}

export interface Metadata {
  appDisplayName: string;
  appIdentifier: string;
  deviceName: string;
  integrationName: string;
  platform: string;
  reactNativeVersion: string;
  unstable_isProfilingBuild: boolean;
  unstable_networkInspectionEnabled: boolean;
  unstable_frameRecordingEnabled: boolean;
  unstable_networkThrottlingEnabled: boolean;
}

export type SessionState =
  "idle" | "connecting" | "attached" | "reconnecting" | "evicted" | "closed";

export interface SessionSnapshot {
  state: SessionState;
  target?: Target | null;
  metadata?: Metadata | null;
  reason?: string;
  attempts: number;
  since: string;
}

/** Mirrors Runtime.RemoteObject (subset Lens decodes). */
export interface RemoteObject {
  type: string;
  subtype?: string;
  className?: string;
  value?: unknown;
  unserializableValue?: string;
  description?: string;
  objectId?: string;
  preview?: ObjectPreview | null;
}

export interface ObjectPreview {
  type: string;
  subtype?: string;
  description?: string;
  overflow: boolean;
  properties: Array<{ name: string; type: string; value?: string; subtype?: string }>;
}

export interface CallFrame {
  functionName: string;
  scriptId: string;
  url: string;
  lineNumber: number;
  columnNumber: number;
}

export interface StackTrace {
  description?: string;
  callFrames: CallFrame[];
  parent?: StackTrace | null;
}

export type ConsoleKind =
  | "log"
  | "debug"
  | "info"
  | "warning"
  | "error"
  | "dir"
  | "dirxml"
  | "table"
  | "trace"
  | "clear"
  | "startGroup"
  | "startGroupCollapsed"
  | "endGroup"
  | "assert"
  | "count"
  | "timeEnd"
  | "exception"
  | "command"
  | "result"
  | "system";

export type ConsoleLevel = "verbose" | "info" | "warning" | "error" | "system";
export type ConsoleSource = "console" | "exception" | "log" | "eval" | "system";

export interface ConsoleEntry {
  seq: number;
  timestamp: string;
  kind: ConsoleKind;
  source: ConsoleSource;
  level: ConsoleLevel;
  text: string;
  args?: RemoteObject[] | null;
  stack?: StackTrace | null;
  location?: { url: string; line: number; column: number } | null;
  contextId?: number;
  epoch: number;
}

export interface ConsoleStats {
  lastSeq: number;
  oldestSeq: number;
  dropped: number;
  len: number;
  epoch: number;
  /** Clear generation: changes whenever the Go log was cleared. */
  cleared: number;
}

export interface ConsoleProperty {
  name: string;
  value?: RemoteObject | null;
  summary: string;
  isOwn: boolean;
  enumerable: boolean;
  writable: boolean;
  hasGetter: boolean;
}

export interface SdkDevice {
  key: string;
  sessionId: string;
  app: { id: string; name?: string; platform: string; rnVersion?: string };
  device: { id: string; name?: string };
  sdkVersion: string;
  features: string[];
  connectedAt: string;
  online: boolean;
}

export type QueryLabel = "fetching" | "fresh" | "stale" | "inactive" | "paused";

export interface QueryState {
  status: "pending" | "error" | "success";
  fetchStatus: "fetching" | "paused" | "idle";
  dataUpdatedAt: number;
  errorUpdatedAt: number;
  dataUpdateCount: number;
  errorUpdateCount: number;
  fetchFailureCount: number;
  isInvalidated: boolean;
  data?: unknown;
  error?: unknown;
  fetchMeta?: unknown;
}

export interface QueryItem {
  queryHash: string;
  queryKey: unknown[];
  state: QueryState;
  observersCount: number;
  isActive: boolean;
  isStale: boolean;
  gcTime?: number | null;
  meta?: unknown;
  dataTruncated?: boolean;
  label: QueryLabel;
}

export interface QueryCounts {
  fetching: number;
  fresh: number;
  stale: number;
  inactive: number;
  paused: number;
  total: number;
}

export interface MutationItem {
  mutationId: number;
  mutationKey?: unknown[] | null;
  state: {
    status: "idle" | "pending" | "success" | "error";
    submittedAt: number;
    isPaused: boolean;
    variables?: unknown;
    data?: unknown;
    error?: unknown;
    failureCount?: number;
    failureReason?: unknown;
    context?: unknown;
  };
  scope?: unknown;
  meta?: unknown;
  dataTruncated?: boolean;
}

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

/** Change-notification topics emitted by Go. Payloads carry no data. */
export const TOPICS = {
  targetsChanged: "targets:changed",
  sessionChanged: "session:changed",
  consoleChanged: "console:changed",
  queriesChanged: "queries:changed",
} as const;

export type Unsubscribe = () => void;

export interface Backend {
  /** Which transport is active; useful for diagnostics and conditional UI. */
  readonly kind: "wails" | "mock";

  app: {
    info(): Promise<AppInfo>;
  };

  targets: {
    list(): Promise<Target[]>;
    refresh(): Promise<void>;
    session(): Promise<SessionSnapshot>;
    attach(id: string): Promise<void>;
    detach(): Promise<void>;
    reload(): Promise<void>;
    openInReactNativeDevTools(id: string): Promise<void>;
  };

  console: {
    stats(): Promise<ConsoleStats>;
    range(fromSeq: number, limit: number): Promise<ConsoleEntry[]>;
    clear(): Promise<void>;
    evaluate(expression: string): Promise<ConsoleEntry>;
    properties(objectId: string, epoch: number): Promise<ConsoleProperty[]>;
  };

  queries: {
    sdkAddr(): Promise<string>;
    devices(): Promise<SdkDevice[]>;
    queries(deviceKey: string): Promise<{ queries: QueryItem[]; counts: QueryCounts }>;
    query(deviceKey: string, queryHash: string): Promise<QueryItem>;
    mutations(deviceKey: string): Promise<MutationItem[]>;
    action(
      deviceKey: string,
      type: QueryActionType,
      queryHash: string,
      dataJSON: string,
    ): Promise<void>;
    setOnline(deviceKey: string, online: boolean): Promise<void>;
  };

  clipboard: {
    /** Puts plain text on the system clipboard. Rejects when the text did not land there. */
    writeText(text: string): Promise<void>;
  };

  events: {
    /** Subscribes to a named Go event. Returns an unsubscribe function. */
    on<T = unknown>(name: string, handler: (data: T) => void): Unsubscribe;
  };
}
