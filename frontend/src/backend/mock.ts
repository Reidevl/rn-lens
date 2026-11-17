import {
  TOPICS,
  type AppInfo,
  type Backend,
  type ConsoleEntry,
  type ConsoleProperty,
  type ConsoleStats,
  type SessionSnapshot,
  type Target,
  type Unsubscribe,
} from "./Backend";

type Handler = (data: unknown) => void;

export interface MockBackend extends Backend {
  /** Emits a Go-style event to every subscriber; used by tests and browser dev. */
  emit(name: string, data: unknown): void;
  /** Replaces the discovered targets and notifies subscribers. */
  setTargets(targets: Target[]): void;
  /** Replaces the session snapshot and notifies subscribers. */
  setSession(session: SessionSnapshot): void;
  /** Appends console entries (seq assigned) and notifies subscribers. */
  log(...entries: Array<Partial<ConsoleEntry> & { text: string }>): ConsoleEntry[];
  /** Simulates a JavaScript context reset: bumps the epoch and logs a system entry. */
  resetContext(): void;
  /** Properties returned by `console.properties` for an objectId. */
  setProperties(objectId: string, props: ConsoleProperty[]): void;
  /** Every method call, in order, for assertions. */
  readonly calls: string[];
}

export interface MockBackendOptions {
  info?: Partial<AppInfo>;
  targets?: Target[];
  session?: SessionSnapshot;
  /** Console capacity before the oldest entries are dropped. */
  consoleCapacity?: number;
}

export const idleSession: SessionSnapshot = {
  state: "idle",
  attempts: 0,
  since: "2026-01-01T00:00:00Z",
};

export function sampleTarget(overrides: Partial<Target> = {}): Target {
  return {
    id: "http://localhost:8081|device-1",
    endpoint: "http://localhost:8081",
    metroId: "device-1",
    pageId: "1",
    title: "host.exp.Exponent (iPhone 17)",
    appId: "host.exp.Exponent",
    deviceName: "iPhone 17",
    logicalDeviceId: "device",
    webSocketUrl: "ws://localhost:8081/inspector/debug?device=device&page=1",
    capabilities: {
      nativePageReloads: true,
      nativeSourceCodeFetching: false,
      supportsMultipleDebuggers: true,
    },
    reloadable: true,
    ...overrides,
  };
}

/** In-memory Backend for tests and for running the UI in a plain browser. */
export function createMockBackend(options: MockBackendOptions = {}): MockBackend {
  const info: AppInfo = { name: "Lens", version: "0.0.0-mock", ...options.info };
  let targets: Target[] = options.targets ?? [];
  let session: SessionSnapshot = options.session ?? idleSession;
  const listeners = new Map<string, Set<Handler>>();
  const calls: string[] = [];
  const consoleCapacity = options.consoleCapacity ?? 10_000;
  let entries: ConsoleEntry[] = [];
  let nextSeq = 1;
  let dropped = 0;
  let epoch = 1;
  const propertyTable = new Map<string, ConsoleProperty[]>();

  const consoleStats = (): ConsoleStats => ({
    lastSeq: nextSeq - 1,
    oldestSeq: entries[0]?.seq ?? 0,
    dropped,
    len: entries.length,
    epoch,
  });
  const appendEntries = (
    items: Array<Partial<ConsoleEntry> & { text: string }>,
  ): ConsoleEntry[] => {
    const added = items.map((item) => {
      const kind = item.kind ?? "log";
      const entry: ConsoleEntry = {
        seq: nextSeq++,
        timestamp: new Date().toISOString(),
        kind,
        source: item.source ?? "console",
        level: item.level ?? levelOf(kind),
        epoch,
        ...item,
      };
      return entry;
    });
    entries.push(...added);
    while (entries.length > consoleCapacity) {
      entries.shift();
      dropped++;
    }
    return added;
  };

  const emit = (name: string, data: unknown) => {
    listeners.get(name)?.forEach((handler) => handler(data));
  };

  const backend: MockBackend = {
    kind: "mock",
    calls,
    app: {
      info: () => Promise.resolve(info),
    },
    targets: {
      list: () => {
        calls.push("list");
        return Promise.resolve(targets);
      },
      refresh: () => {
        calls.push("refresh");
        return Promise.resolve();
      },
      session: () => {
        calls.push("session");
        return Promise.resolve(session);
      },
      attach: (id) => {
        calls.push(`attach:${id}`);
        const target = targets.find((t) => t.id === id);
        if (!target) return Promise.reject(new Error(`unknown target ${id}`));
        backend.setSession({ ...idleSession, state: "attached", target });
        return Promise.resolve();
      },
      detach: () => {
        calls.push("detach");
        backend.setSession({ ...idleSession, state: "closed", reason: "detached" });
        return Promise.resolve();
      },
      reload: () => {
        calls.push("reload");
        return Promise.resolve();
      },
      openInReactNativeDevTools: (id) => {
        calls.push(`open:${id}`);
        return Promise.resolve();
      },
    },
    console: {
      stats: () => {
        calls.push("console.stats");
        return Promise.resolve(consoleStats());
      },
      range: (fromSeq, limit) => {
        calls.push(`console.range:${fromSeq}:${limit}`);
        const slice = entries.filter((e) => e.seq >= fromSeq);
        return Promise.resolve(limit > 0 ? slice.slice(0, limit) : slice);
      },
      clear: () => {
        calls.push("console.clear");
        entries = [];
        dropped = 0;
        emit(TOPICS.consoleChanged, { topic: TOPICS.consoleChanged, seq: Date.now() });
        return Promise.resolve();
      },
      evaluate: (expression) => {
        calls.push(`console.evaluate:${expression}`);
        appendEntries([{ kind: "command", source: "eval", text: expression }]);
        let result: Partial<ConsoleEntry> & { text: string };
        if (expression === "throw") {
          result = { kind: "exception", source: "eval", text: "Error: thrown from mock" };
        } else {
          let text: string;
          try {
            text = String(new Function(`return (${expression})`)());
          } catch (err) {
            text = err instanceof Error ? `${err.name}: ${err.message}` : String(err);
          }
          result = {
            kind: "result",
            source: "eval",
            text,
            args: [{ type: "number", value: Number(text) }],
          };
        }
        const [entry] = appendEntries([result]);
        emit(TOPICS.consoleChanged, { topic: TOPICS.consoleChanged, seq: Date.now() });
        return Promise.resolve(entry);
      },
      properties: (objectId, entryEpoch) => {
        calls.push(`console.properties:${objectId}`);
        if (entryEpoch < epoch) {
          return Promise.reject(
            new Error("console: object is no longer available after the context was reset"),
          );
        }
        return Promise.resolve(propertyTable.get(objectId) ?? []);
      },
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
    emit,
    setTargets: (next) => {
      targets = next;
      emit(TOPICS.targetsChanged, { topic: TOPICS.targetsChanged, seq: Date.now() });
    },
    setSession: (next) => {
      session = next;
      emit(TOPICS.sessionChanged, { topic: TOPICS.sessionChanged, seq: Date.now() });
    },
    log: (...items) => {
      const added = appendEntries(items);
      emit(TOPICS.consoleChanged, { topic: TOPICS.consoleChanged, seq: Date.now() });
      return added;
    },
    resetContext: () => {
      epoch++;
      appendEntries([
        { kind: "system", source: "system", text: "JavaScript context was reset (app reloaded)" },
      ]);
      emit(TOPICS.consoleChanged, { topic: TOPICS.consoleChanged, seq: Date.now() });
    },
    setProperties: (objectId, props) => {
      propertyTable.set(objectId, props);
    },
  };
  return backend;
}

function levelOf(kind: ConsoleEntry["kind"]): ConsoleEntry["level"] {
  switch (kind) {
    case "debug":
    case "trace":
    case "count":
    case "timeEnd":
      return "verbose";
    case "warning":
      return "warning";
    case "error":
    case "exception":
    case "assert":
      return "error";
    case "system":
    case "command":
    case "result":
      return "system";
    default:
      return "info";
  }
}
