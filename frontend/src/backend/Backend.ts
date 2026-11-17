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

/** Change-notification topics emitted by Go. Payloads carry no data. */
export const TOPICS = {
  targetsChanged: "targets:changed",
  sessionChanged: "session:changed",
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

  events: {
    /** Subscribes to a named Go event. Returns an unsubscribe function. */
    on<T = unknown>(name: string, handler: (data: T) => void): Unsubscribe;
  };
}
