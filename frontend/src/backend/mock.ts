import type { AppInfo, Backend, Unsubscribe } from "./Backend";

type Handler = (data: unknown) => void;

export interface MockBackend extends Backend {
  /** Emits a Go-style event to every subscriber; used by tests and browser dev. */
  emit(name: string, data: unknown): void;
}

export interface MockBackendOptions {
  info?: Partial<AppInfo>;
}

/** In-memory Backend for tests and for running the UI in a plain browser. */
export function createMockBackend(options: MockBackendOptions = {}): MockBackend {
  const info: AppInfo = { name: "Lens", version: "0.0.0-mock", ...options.info };
  const listeners = new Map<string, Set<Handler>>();

  return {
    kind: "mock",
    app: {
      info: () => Promise.resolve(info),
    },
    events: {
      on: (name, handler): Unsubscribe => {
        const set = listeners.get(name) ?? new Set<Handler>();
        set.add(handler as Handler);
        listeners.set(name, set);
        return () => {
          set.delete(handler as Handler);
        };
      },
    },
    emit: (name, data) => {
      listeners.get(name)?.forEach((handler) => handler(data));
    },
  };
}
