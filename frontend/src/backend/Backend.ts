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

export type NetworkState = "pending" | "complete" | "failed";

/** Whether the attached app supports network inspection (React Native 0.83+). */
export interface NetworkAvailability {
  /** False until the first attach finished enabling the Network domain. */
  checked: boolean;
  supported: boolean;
  reason?: string;
}

/** Position of Go's request store; pull rows changed after `version`. */
export interface NetworkStats {
  version: number;
  len: number;
  oldestSeq: number;
  lastSeq: number;
  dropped: number;
  epoch: number;
  cleared: number;
}

/** One table row. Times are CDP monotonic seconds; startedAt is epoch ms. */
export interface NetworkSummary {
  seq: number;
  epoch: number;
  version: number;
  requestId: string;
  url: string;
  method: string;
  resourceType: string;
  state: NetworkState;
  status: number;
  statusText: string;
  mimeType: string;
  startedAt: number;
  startTime: number;
  responseTime: number;
  endTime: number;
  encodedSize: number;
  decodedSize: number;
  errorText?: string;
  canceled?: boolean;
  redirected?: boolean;
  fromCache?: boolean;
}

/** CDP ResourceTiming: phases in ms after requestTime (seconds); -1 = none. */
export interface NetworkTiming {
  requestTime: number;
  proxyStart: number;
  proxyEnd: number;
  dnsStart: number;
  dnsEnd: number;
  connectStart: number;
  connectEnd: number;
  sslStart: number;
  sslEnd: number;
  sendStart: number;
  sendEnd: number;
  receiveHeadersEnd: number;
}

export interface NetworkInitiator {
  type: string;
  url?: string;
  lineNumber?: number;
  stack?: unknown;
}

export interface NetworkRequest extends NetworkSummary {
  requestHeaders: Record<string, string> | null;
  responseHeaders: Record<string, string> | null;
  hasPostData: boolean;
  postData?: string;
  protocol?: string;
  remoteAddress?: string;
  timing?: NetworkTiming | null;
  initiator?: NetworkInitiator | null;
}

/** A response body; `text` is base64 when `base64` is set. */
export interface NetworkBody {
  text: string;
  base64: boolean;
  truncated: boolean;
  size: number;
}

export type StorageKind = "async-storage" | "mmkv" | "secure-store";
export type StorageValueType = "string" | "number" | "boolean" | "bytes";

/** A connected SDK that exposes storage. */
export interface StorageDevice {
  key: string;
  sessionId: string;
  app: { id: string; name?: string; platform: string; rnVersion?: string };
  device: { id: string; name?: string };
  sdkVersion: string;
  connectedAt: string;
}

export interface StorageKeyInfo {
  key: string;
  /** Bytes of the value, when the SDK can tell cheaply. */
  size?: number;
}

export interface StorageStore {
  id: string;
  kind: StorageKind;
  name: string;
  keys: StorageKeyInfo[] | null;
  /** More keys exist than the snapshot carries. */
  truncated?: boolean;
  /** Changes arrive as they happen (MMKV). */
  live?: boolean;
  /** Only the keys the app declared are listed (SecureStore). */
  declaredKeys?: boolean;
}

/** A value: strings as is, numbers and booleans in JSON form, bytes as base64. */
export interface StorageValue {
  type: StorageValueType;
  value: string;
  truncated?: boolean;
  size?: number;
}

export interface StorageImportOutcome {
  /** "" when the user canceled. */
  path: string;
  written: number;
  skipped: Record<string, string> | null;
}

/** A device whose app registered plugin panels. */
export type PluginDevice = StorageDevice;

/** A button of a plugin panel; clicking it sends the action to the app. */
export interface PluginButton {
  id: string;
  label: string;
  destructive?: boolean;
  /** Lens asks the user to confirm this text first. */
  confirm?: string;
}

interface PluginBlockBase {
  id: string;
  title?: string;
}

/** One building block of a plugin panel (see protocol/README.md). */
export type PluginBlock =
  | (PluginBlockBase & {
      type: "text";
      text: string;
      tone?: "default" | "muted" | "warning" | "error";
    })
  | (PluginBlockBase & {
      type: "keyValue";
      items: { key: string; label?: string; value: unknown }[];
    })
  | (PluginBlockBase & {
      type: "table";
      columns: { key: string; label: string }[];
      rows: { id: string; cells: Record<string, unknown> }[];
      rowActions?: PluginButton[];
    })
  | (PluginBlockBase & { type: "json"; value: unknown })
  | (PluginBlockBase & {
      type: "log";
      entries: { text: string; level?: "debug" | "info" | "warn" | "error"; time?: string }[];
    })
  | (PluginBlockBase & { type: "actions"; actions: PluginButton[] });

/** A plugin of the app and its current panel. */
export interface PluginInfo {
  id: string;
  name: string;
  description?: string;
  version?: string;
  /** The last view; a block of a kind this Lens does not know is kept as is. */
  blocks: PluginBlock[];
  /** Why the last view is not shown (too large, too many blocks). */
  error?: string;
  updatedAt?: string;
}

/** Where the last Fast Refresh stands. */
export type FastRefreshPhase =
  "off" | "unavailable" | "connecting" | "ready" | "updating" | "updated" | "error";

export interface FastRefreshState {
  phase: FastRefreshPhase;
  /** Modules added, changed or deleted by the last update. */
  modules?: number;
  /** ISO time of the last update or error. */
  at?: string;
  /** Explains "error" and "unavailable". */
  message?: string;
}

export interface PerformanceStatus {
  tracing: boolean;
  profiling: boolean;
  snapshotting: boolean;
  fastRefresh: FastRefreshState;
}

export type ScreenshotFormat = "png" | "jpeg";

export interface Screenshot {
  format: ScreenshotFormat;
  /** The image, base64-encoded. */
  data: string;
  bytes: number;
}

export interface TraceSummary {
  events: number;
  durationMs: number;
  /** Size of the exported file. */
  bytes: number;
  dataLoss: boolean;
}

export interface ProfileSummary {
  nodes: number;
  samples: number;
  durationMs: number;
  bytes: number;
}

export interface HeapSnapshotResult {
  /** "" when the user canceled. */
  path: string;
  bytes: number;
}

/** Change-notification topics emitted by Go. Payloads carry no data. */
export const TOPICS = {
  targetsChanged: "targets:changed",
  sessionChanged: "session:changed",
  consoleChanged: "console:changed",
  queriesChanged: "queries:changed",
  networkChanged: "network:changed",
  storageChanged: "storage:changed",
  performanceChanged: "performance:changed",
  pluginsChanged: "plugins:changed",
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

  network: {
    availability(): Promise<NetworkAvailability>;
    stats(): Promise<NetworkStats>;
    /** Rows changed after `version`, oldest change first; `limit` 0 = all. */
    since(version: number, limit: number): Promise<NetworkSummary[]>;
    request(seq: number): Promise<NetworkRequest>;
    responseBody(seq: number, epoch: number): Promise<NetworkBody>;
    requestBody(seq: number, epoch: number): Promise<string>;
    clear(): Promise<void>;
    /** Asks where to save and writes a HAR; resolves to the path, or "" if canceled. */
    saveHAR(): Promise<string>;
  };

  storage: {
    devices(): Promise<StorageDevice[]>;
    stores(deviceKey: string): Promise<StorageStore[]>;
    get(deviceKey: string, storeId: string, key: string): Promise<StorageValue>;
    set(
      deviceKey: string,
      storeId: string,
      key: string,
      type: StorageValueType,
      value: string,
    ): Promise<void>;
    remove(deviceKey: string, storeId: string, key: string): Promise<void>;
    clear(deviceKey: string, storeId: string): Promise<void>;
    refresh(deviceKey: string, storeId: string): Promise<void>;
    /** Asks where to save; resolves to the path, or "" if canceled. */
    exportStore(deviceKey: string, storeId: string): Promise<string>;
    /** Asks for an export file and writes it into the store. */
    importStore(deviceKey: string, storeId: string): Promise<StorageImportOutcome>;
  };

  performance: {
    status(): Promise<PerformanceStatus>;
    /** Categories recorded when startTrace gets none. */
    traceCategories(): Promise<string[]>;
    /** `quality` (1-100) applies to JPEG only. */
    screenshot(format: ScreenshotFormat, quality: number): Promise<Screenshot>;
    /** Saves the last screenshot; resolves to the path, or "" if canceled. */
    saveScreenshot(): Promise<string>;
    startTrace(categories: string[]): Promise<void>;
    stopTrace(): Promise<TraceSummary>;
    /** Saves the last trace as Chrome JSON; resolves to the path, or "" if canceled. */
    saveTrace(): Promise<string>;
    startProfile(): Promise<void>;
    stopProfile(): Promise<ProfileSummary>;
    /** Saves the last CPU profile; resolves to the path, or "" if canceled. */
    saveProfile(): Promise<string>;
    /** Asks where to save, then streams a heap snapshot there. */
    saveHeapSnapshot(): Promise<HeapSnapshotResult>;
  };

  plugins: {
    devices(): Promise<PluginDevice[]>;
    plugins(deviceKey: string): Promise<PluginInfo[]>;
    /** Sends a click to the app; resolves to the app's message ("" when none). */
    action(
      deviceKey: string,
      pluginId: string,
      blockId: string,
      actionId: string,
      rowId?: string,
    ): Promise<string>;
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
