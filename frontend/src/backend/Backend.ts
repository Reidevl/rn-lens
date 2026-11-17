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

export type Unsubscribe = () => void;

export interface Backend {
  /** Which transport is active; useful for diagnostics and conditional UI. */
  readonly kind: "wails" | "mock";

  app: {
    info(): Promise<AppInfo>;
  };

  events: {
    /** Subscribes to a named Go event. Returns an unsubscribe function. */
    on<T = unknown>(name: string, handler: (data: T) => void): Unsubscribe;
  };
}
