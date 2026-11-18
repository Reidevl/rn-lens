import {
  TOPICS,
  type AppInfo,
  type MCPInfo,
  type McpCall,
  type McpTool,
  type Backend,
  type ConsoleEntry,
  type ConsoleProperty,
  type ConsoleStats,
  type MutationItem,
  type NetworkAvailability,
  type NetworkBody,
  type NetworkRequest,
  type PerformanceStatus,
  type PluginDevice,
  type PluginInfo,
  type QueryCounts,
  type QueryItem,
  type QueryLabel,
  type RemoteObject,
  type SdkDevice,
  type SessionSnapshot,
  type StorageDevice,
  type StorageStore,
  type StorageValue,
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
  /**
   * Appends console entries and notifies subscribers. `seq` and `epoch` are
   * always assigned by the mock; passing them has no effect.
   */
  log(...entries: Array<Partial<ConsoleEntry> & { text: string }>): ConsoleEntry[];
  /** Simulates a JavaScript context reset: bumps the epoch and logs a system entry. */
  resetContext(): void;
  /** Properties returned by `console.properties` for an objectId. */
  setProperties(objectId: string, props: ConsoleProperty[]): void;
  /** Connects a fake SDK device with queries and mutations, and notifies. */
  connectDevice(device: SdkDevice, queries?: QueryItem[], mutations?: MutationItem[]): void;
  /** Replaces a device's queries and notifies. */
  setQueries(deviceKey: string, queries: QueryItem[]): void;
  /** Disconnects a device and notifies. */
  disconnectDevice(deviceKey: string): void;
  /** Every method call, in order, for assertions. */
  readonly calls: string[];
  /** Sets what `network.availability` reports and notifies. */
  setNetworkAvailability(availability: NetworkAvailability): void;
  /** Adds a captured request (defaults fill the rest) and notifies. */
  addRequest(request: Partial<NetworkRequest> & { url: string }): NetworkRequest;
  /** Changes a captured request, bumping its version, and notifies. */
  updateRequest(seq: number, patch: Partial<NetworkRequest>): void;
  /** Body returned by `network.responseBody` for a request. */
  setResponseBody(seq: number, body: NetworkBody | string): void;
  /** Starts a new network epoch (a re-attach) and notifies. */
  resetNetworkEpoch(): void;
  /** Connects a fake SDK device with stores and their values, and notifies. */
  connectStorage(
    device: StorageDevice,
    stores: StorageStore[],
    values?: Record<string, StorageValue>,
  ): void;
  /** Current value of `<storeId>/<key>` on the fake device, for assertions. */
  storageValue(deviceKey: string, storeId: string, key: string): StorageValue | undefined;
  /** Patches what `performance.status` reports and notifies. */
  setPerformance(patch: Partial<PerformanceStatus>): void;
  /** Makes every `performance` call reject with `message`; null restores them. */
  failPerformance(message: string | null): void;
  /** Connects a fake device with plugins (or replaces them) and notifies. */
  setPlugins(device: PluginDevice, plugins: PluginInfo[]): void;
  /** Disconnects a plugin device and notifies. */
  removePluginDevice(deviceKey: string): void;
  /**
   * Answers `plugins.action`: return the app's message or throw its error.
   * Defaults to answering "".
   */
  onPluginAction(
    handler: (action: {
      deviceKey: string;
      pluginId: string;
      blockId: string;
      actionId: string;
      rowId?: string;
    }) => string | Promise<string>,
  ): void;
  /** Records an AI tool call (seq and time are filled in) and notifies. */
  recordMcpCall(call: Partial<McpCall> & { tool: string }): McpCall;
  /** Text last written through `clipboard.writeText`. */
  readonly clipboardText: string;
  /** Makes `clipboard.writeText` reject, to exercise the failure path. */
  failClipboard(fail: boolean): void;
}

export interface MockBackendOptions {
  info?: Partial<AppInfo>;
  /** What `app.mcp` reports; MCP is off when omitted. */
  mcp?: MCPInfo;
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
  let clipboardText = "";
  let clipboardFails = false;
  const consoleCapacity = options.consoleCapacity ?? 10_000;
  let entries: ConsoleEntry[] = [];
  let nextSeq = 1;
  let dropped = 0;
  let epoch = 1;
  let cleared = 0;
  const propertyTable = new Map<string, ConsoleProperty[]>();
  let networkAvailability: NetworkAvailability = { checked: true, supported: true };
  const requests = new Map<number, NetworkRequest>();
  const responseBodies = new Map<number, NetworkBody>();
  let netVersion = 0;
  let netSeq = 0;
  let netEpoch = 1;
  let netCleared = 0;
  const netDropped = 0;
  const storageDevices = new Map<
    string,
    { device: StorageDevice; stores: StorageStore[]; values: Map<string, StorageValue> }
  >();
  let perfStatus: PerformanceStatus = {
    tracing: false,
    profiling: false,
    snapshotting: false,
    fastRefresh: { phase: "off" },
  };
  let perfFailure: string | null = null;
  let mcpReadOnly = false;
  let mcpCalls: McpCall[] = [];
  let mcpSeq = 0;
  const mcpTools: McpTool[] = [
    { name: "status", description: "Lens version and the attached app.", kind: "read" },
    { name: "set_online", description: "Switch onlineManager.", kind: "change" },
    { name: "clear_storage", description: "Delete every key of a store.", kind: "destructive" },
  ];
  const notifyMcp = () => emit(TOPICS.mcpChanged, { topic: TOPICS.mcpChanged, seq: Date.now() });
  const pluginDevices = new Map<string, { device: PluginDevice; plugins: PluginInfo[] }>();
  let pluginActionHandler: Parameters<MockBackend["onPluginAction"]>[0] = () => "";
  const notifyPlugins = () =>
    emit(TOPICS.pluginsChanged, { topic: TOPICS.pluginsChanged, seq: Date.now() });
  const notifyPerformance = () =>
    emit(TOPICS.performanceChanged, { topic: TOPICS.performanceChanged, seq: Date.now() });
  /** Records the call and fails it when failPerformance is set. */
  const perfCall = (name: string) => {
    calls.push(`performance.${name}`);
    if (perfFailure !== null) throw new Error(perfFailure);
  };
  const setPerf = (patch: Partial<PerformanceStatus>) => {
    perfStatus = { ...perfStatus, ...patch };
    notifyPerformance();
  };
  const notifyStorage = () =>
    emit(TOPICS.storageChanged, { topic: TOPICS.storageChanged, seq: Date.now() });
  const storageOf = (deviceKey: string, storeId: string) => {
    const d = storageDevices.get(deviceKey);
    if (!d) throw new Error("storage: unknown device");
    const store = d.stores.find((s) => s.id === storeId);
    if (!store) throw new Error("storage: unknown store");
    return { d, store };
  };
  const setKeys = (store: StorageStore, keys: string[]) => {
    store.keys = [...new Set(keys)].sort().map((key) => ({ key }));
  };
  const notifyNetwork = () =>
    emit(TOPICS.networkChanged, { topic: TOPICS.networkChanged, seq: Date.now() });
  const devices = new Map<
    string,
    { device: SdkDevice; queries: QueryItem[]; mutations: MutationItem[] }
  >();
  const notifyQueries = () =>
    emit(TOPICS.queriesChanged, { topic: TOPICS.queriesChanged, seq: Date.now() });
  const countQueries = (list: QueryItem[]): QueryCounts => {
    const c: QueryCounts = {
      fetching: 0,
      fresh: 0,
      stale: 0,
      inactive: 0,
      paused: 0,
      total: list.length,
    };
    for (const q of list) c[q.label]++;
    return c;
  };

  const consoleStats = (): ConsoleStats => ({
    lastSeq: nextSeq - 1,
    oldestSeq: entries[0]?.seq ?? 0,
    dropped,
    len: entries.length,
    epoch,
    cleared,
  });
  const appendEntries = (
    items: Array<Partial<ConsoleEntry> & { text: string }>,
  ): ConsoleEntry[] => {
    const added = items.map((item) => {
      const kind = item.kind ?? "log";
      const entry: ConsoleEntry = {
        timestamp: new Date().toISOString(),
        ...item,
        kind,
        source: item.source ?? "console",
        level: item.level ?? levelOf(kind),
        // Mock-owned fields come last so a caller cannot override them.
        seq: nextSeq++,
        epoch,
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
      mcp: () => Promise.resolve(options.mcp ?? { url: "" }),
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
        cleared++;
        emit(TOPICS.consoleChanged, { topic: TOPICS.consoleChanged, seq: Date.now() });
        return Promise.resolve();
      },
      evaluate: (expression) => {
        calls.push(`console.evaluate:${expression}`);
        appendEntries([{ kind: "command", source: "eval", text: expression }]);
        const outcome = evaluateStatic(expression);
        const result: Partial<ConsoleEntry> & { text: string } =
          "error" in outcome
            ? { kind: "exception", source: "eval", text: outcome.error }
            : {
                kind: "result",
                source: "eval",
                text: describeValue(outcome.value),
                args: [remoteObjectOf(outcome.value)],
              };
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
    queries: {
      sdkAddr: () => Promise.resolve("127.0.0.1:7420"),
      devices: () => {
        calls.push("queries.devices");
        return Promise.resolve([...devices.values()].map((d) => d.device));
      },
      queries: (deviceKey) => {
        calls.push(`queries.queries:${deviceKey}`);
        const d = devices.get(deviceKey);
        if (!d) return Promise.reject(new Error("queries: unknown device"));
        return Promise.resolve({ queries: d.queries, counts: countQueries(d.queries) });
      },
      query: (deviceKey, queryHash) => {
        const q = devices.get(deviceKey)?.queries.find((x) => x.queryHash === queryHash);
        return q ? Promise.resolve(q) : Promise.reject(new Error(`queries: no query ${queryHash}`));
      },
      mutations: (deviceKey) => {
        const d = devices.get(deviceKey);
        return d
          ? Promise.resolve(d.mutations)
          : Promise.reject(new Error("queries: unknown device"));
      },
      action: (deviceKey, type, queryHash, dataJSON) => {
        calls.push(`queries.action:${type}:${queryHash}`);
        const d = devices.get(deviceKey);
        if (!d) return Promise.reject(new Error("queries: unknown device"));
        const q = d.queries.find((x) => x.queryHash === queryHash);
        const relabel = (label: QueryLabel) => {
          if (q) q.label = label;
        };
        switch (type) {
          case "INVALIDATE":
            relabel("stale");
            if (q) q.state = { ...q.state, isInvalidated: true };
            break;
          case "REFETCH":
            relabel("fetching");
            break;
          case "REMOVE":
            d.queries = d.queries.filter((x) => x.queryHash !== queryHash);
            break;
          case "SET_QUERY_DATA":
            if (q) q.state = { ...q.state, data: JSON.parse(dataJSON) };
            break;
          case "TRIGGER_ERROR":
            if (q)
              q.state = {
                ...q.state,
                status: "error",
                error: { message: "Unknown error from devtools" },
              };
            break;
          case "CLEAR_QUERY_CACHE":
            d.queries = [];
            break;
          case "CLEAR_MUTATION_CACHE":
            d.mutations = [];
            break;
          default:
            break;
        }
        notifyQueries();
        return Promise.resolve();
      },
      setOnline: (deviceKey, online) => {
        calls.push(`queries.setOnline:${online}`);
        const d = devices.get(deviceKey);
        if (!d) return Promise.reject(new Error("queries: unknown device"));
        d.device = { ...d.device, online };
        notifyQueries();
        return Promise.resolve();
      },
    },
    network: {
      availability: () => Promise.resolve(networkAvailability),
      stats: () => {
        const seqs = [...requests.keys()].sort((a, b) => a - b);
        return Promise.resolve({
          version: netVersion,
          len: requests.size,
          oldestSeq: seqs[0] ?? 0,
          lastSeq: netSeq,
          dropped: netDropped,
          epoch: netEpoch,
          cleared: netCleared,
        });
      },
      since: (version, limit) => {
        calls.push(`network.since:${version}`);
        const rows = [...requests.values()]
          .filter((r) => r.version > version)
          .sort((a, b) => a.version - b.version)
          .map(
            ({
              requestHeaders: _rq,
              responseHeaders: _rs,
              hasPostData: _h,
              postData: _p,
              protocol: _pr,
              remoteAddress: _ra,
              timing: _t,
              initiator: _i,
              ...summary
            }) => summary,
          );
        return Promise.resolve(limit > 0 ? rows.slice(0, limit) : rows);
      },
      request: (seq) => {
        const r = requests.get(seq);
        return r
          ? Promise.resolve({ ...r })
          : Promise.reject(new Error("network: request not found"));
      },
      responseBody: (seq, epoch) => {
        calls.push(`network.responseBody:${seq}`);
        if (epoch !== netEpoch) return Promise.reject(new Error("network: earlier session"));
        const body = responseBodies.get(seq);
        return body ? Promise.resolve(body) : Promise.reject(new Error("network: no body"));
      },
      requestBody: (seq, epoch) =>
        epoch === netEpoch
          ? Promise.resolve(requests.get(seq)?.postData ?? "")
          : Promise.reject(new Error("network: earlier session")),
      clear: () => {
        calls.push("network.clear");
        requests.clear();
        responseBodies.clear();
        netCleared += 1;
        netVersion += 1;
        notifyNetwork();
        return Promise.resolve();
      },
      saveHAR: () => {
        calls.push("network.saveHAR");
        return Promise.resolve("/Users/me/lens.har");
      },
    },
    setNetworkAvailability: (availability) => {
      networkAvailability = availability;
      notifyNetwork();
    },
    addRequest: (partial) => {
      netSeq += 1;
      netVersion += 1;
      const request: NetworkRequest = {
        seq: netSeq,
        epoch: netEpoch,
        version: netVersion,
        requestId: `r${netSeq}`,
        method: "GET",
        resourceType: "Fetch",
        state: "pending",
        status: 0,
        statusText: "",
        mimeType: "",
        startedAt: Date.now(),
        startTime: netSeq,
        responseTime: 0,
        endTime: 0,
        encodedSize: 0,
        decodedSize: 0,
        requestHeaders: {},
        responseHeaders: {},
        hasPostData: false,
        ...partial,
      };
      request.seq = netSeq;
      request.version = netVersion;
      request.epoch = partial.epoch ?? netEpoch;
      requests.set(request.seq, request);
      notifyNetwork();
      return request;
    },
    updateRequest: (seq, patch) => {
      const r = requests.get(seq);
      if (!r) return;
      netVersion += 1;
      requests.set(seq, { ...r, ...patch, seq, version: netVersion });
      notifyNetwork();
    },
    setResponseBody: (seq, body) => {
      responseBodies.set(
        seq,
        typeof body === "string"
          ? { text: body, base64: false, truncated: false, size: body.length }
          : body,
      );
    },
    resetNetworkEpoch: () => {
      netEpoch += 1;
      netVersion += 1;
      notifyNetwork();
    },
    storage: {
      devices: () => Promise.resolve([...storageDevices.values()].map((d) => d.device)),
      stores: (deviceKey) => {
        const d = storageDevices.get(deviceKey);
        return d
          ? Promise.resolve(d.stores.map((s) => ({ ...s, keys: [...(s.keys ?? [])] })))
          : Promise.reject(new Error("storage: unknown device"));
      },
      get: async (deviceKey, storeId, key) => {
        calls.push(`storage.get:${storeId}/${key}`);
        const { d } = storageOf(deviceKey, storeId);
        const v = d.values.get(`${storeId}/${key}`);
        if (!v) throw new Error("not set");
        return v;
      },
      set: async (deviceKey, storeId, key, type, value) => {
        calls.push(`storage.set:${storeId}/${key}=${type}:${value}`);
        const { d, store } = storageOf(deviceKey, storeId);
        d.values.set(`${storeId}/${key}`, { type, value });
        setKeys(store, [...(store.keys ?? []).map((k) => k.key), key]);
        notifyStorage();
      },
      remove: async (deviceKey, storeId, key) => {
        calls.push(`storage.remove:${storeId}/${key}`);
        const { d, store } = storageOf(deviceKey, storeId);
        d.values.delete(`${storeId}/${key}`);
        setKeys(
          store,
          (store.keys ?? []).map((k) => k.key).filter((k) => k !== key),
        );
        notifyStorage();
      },
      clear: async (deviceKey, storeId) => {
        calls.push(`storage.clear:${storeId}`);
        const { d, store } = storageOf(deviceKey, storeId);
        for (const k of store.keys ?? []) d.values.delete(`${storeId}/${k.key}`);
        store.keys = [];
        notifyStorage();
      },
      refresh: async (deviceKey, storeId) => {
        calls.push(`storage.refresh:${storeId}`);
        storageOf(deviceKey, storeId);
        notifyStorage();
      },
      exportStore: async (deviceKey, storeId) => {
        calls.push(`storage.export:${storeId}`);
        storageOf(deviceKey, storeId);
        return "/Users/me/lens-async-storage.json";
      },
      importStore: async (deviceKey, storeId) => {
        calls.push(`storage.import:${storeId}`);
        storageOf(deviceKey, storeId);
        return {
          path: "/Users/me/backup.json",
          written: 2,
          skipped: { big: "the export holds only the size of this value" },
        };
      },
    },
    connectStorage: (device, stores, values = {}) => {
      storageDevices.set(device.key, {
        device,
        stores: stores.map((s) => ({ ...s, keys: [...(s.keys ?? [])] })),
        values: new Map(Object.entries(values)),
      });
      notifyStorage();
    },
    storageValue: (deviceKey, storeId, key) =>
      storageDevices.get(deviceKey)?.values.get(`${storeId}/${key}`),
    performance: {
      status: async () => ({ ...perfStatus }),
      traceCategories: async () => ["devtools.timeline", "disabled-by-default-v8.cpu_profiler"],
      screenshot: async (format) => {
        perfCall("screenshot");
        // A 1x1 transparent PNG.
        return {
          format,
          data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
          bytes: 68,
        };
      },
      saveScreenshot: async () => {
        perfCall("saveScreenshot");
        return "/Users/me/lens-screenshot.png";
      },
      startTrace: async (categories) => {
        perfCall(`startTrace:${categories.join(",")}`);
        if (perfStatus.tracing)
          throw new Error("perf: a recording of this kind is already running");
        setPerf({ tracing: true });
      },
      stopTrace: async () => {
        perfCall("stopTrace");
        setPerf({ tracing: false });
        return { events: 1520, durationMs: 4210.5, bytes: 312_000, dataLoss: false };
      },
      saveTrace: async () => {
        perfCall("saveTrace");
        return "/Users/me/lens-trace.json";
      },
      startProfile: async () => {
        perfCall("startProfile");
        setPerf({ profiling: true });
      },
      stopProfile: async () => {
        perfCall("stopProfile");
        setPerf({ profiling: false });
        return { nodes: 840, samples: 3900, durationMs: 5000, bytes: 210_000 };
      },
      saveProfile: async () => {
        perfCall("saveProfile");
        return "/Users/me/lens.cpuprofile";
      },
      saveHeapSnapshot: async () => {
        perfCall("saveHeapSnapshot");
        return { path: "/Users/me/lens.heapsnapshot", bytes: 6_500_000 };
      },
    },
    plugins: {
      devices: async () => [...pluginDevices.values()].map((d) => d.device),
      plugins: async (deviceKey) => {
        const d = pluginDevices.get(deviceKey);
        if (!d) throw new Error("plugins: unknown device");
        return d.plugins.map((p) => ({ ...p, blocks: [...p.blocks] }));
      },
      action: async (deviceKey, pluginId, blockId, actionId, rowId) => {
        calls.push(`plugins.action:${pluginId}/${blockId}/${actionId}${rowId ? `/${rowId}` : ""}`);
        if (!pluginDevices.has(deviceKey)) throw new Error("plugins: unknown device");
        return pluginActionHandler({
          deviceKey,
          pluginId,
          blockId,
          actionId,
          ...(rowId ? { rowId } : {}),
        });
      },
    },
    mcp: {
      status: async () => ({
        enabled: Boolean(options.mcp?.url),
        url: options.mcp?.url ?? "",
        tokenSet: Boolean(options.mcp?.token),
        readOnly: mcpReadOnly,
      }),
      tools: async () => (options.mcp?.url ? [...mcpTools] : []),
      activity: async (after) => mcpCalls.filter((c) => c.seq > after),
      clearActivity: async () => {
        calls.push("mcp.clearActivity");
        mcpCalls = [];
        notifyMcp();
      },
      setReadOnly: async (on) => {
        calls.push(`mcp.setReadOnly:${on}`);
        if (!options.mcp?.url) throw new Error("MCP is off");
        mcpReadOnly = on;
        notifyMcp();
      },
    },
    recordMcpCall: (call) => {
      const full: McpCall = {
        client: "claude-code 2.1",
        kind: "read",
        durationMs: 3,
        time: new Date().toISOString(),
        ...call,
        seq: ++mcpSeq,
      };
      mcpCalls = [...mcpCalls, full];
      notifyMcp();
      return full;
    },
    setPlugins: (device, plugins) => {
      pluginDevices.set(device.key, { device, plugins });
      notifyPlugins();
    },
    removePluginDevice: (deviceKey) => {
      pluginDevices.delete(deviceKey);
      notifyPlugins();
    },
    onPluginAction: (handler) => {
      pluginActionHandler = handler;
    },
    setPerformance: setPerf,
    failPerformance: (message) => {
      perfFailure = message;
    },
    clipboard: {
      writeText: (text) => {
        calls.push("clipboard.writeText");
        if (clipboardFails) return Promise.reject(new Error("clipboard: denied"));
        clipboardText = text;
        return Promise.resolve();
      },
    },
    get clipboardText() {
      return clipboardText;
    },
    failClipboard: (fail) => {
      clipboardFails = fail;
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
    connectDevice: (device, qs = [], ms = []) => {
      devices.set(device.key, { device, queries: qs, mutations: ms });
      notifyQueries();
    },
    setQueries: (deviceKey, qs) => {
      const d = devices.get(deviceKey);
      if (d) d.queries = qs;
      notifyQueries();
    },
    disconnectDevice: (deviceKey) => {
      devices.delete(deviceKey);
      notifyQueries();
    },
  };
  return backend;
}

/** A sample connected SDK device for tests and browser previews. */
export function sampleDevice(overrides: Partial<SdkDevice> = {}): SdkDevice {
  return {
    key: "host.exp.Exponent|device-1",
    sessionId: "s1",
    app: { id: "host.exp.Exponent", name: "Lens Example", platform: "ios", rnVersion: "0.88.1" },
    device: { id: "device-1", name: "iPhone 17" },
    sdkVersion: "0.1.0",
    features: ["queries"],
    connectedAt: "2026-01-01T00:00:00Z",
    online: true,
    ...overrides,
  };
}

/** A sample query in the fresh state. */
export function sampleQuery(overrides: Partial<QueryItem> = {}): QueryItem {
  return {
    queryHash: '["todos",{"page":1}]',
    queryKey: ["todos", { page: 1 }],
    state: {
      status: "success",
      fetchStatus: "idle",
      dataUpdatedAt: Date.now(),
      errorUpdatedAt: 0,
      dataUpdateCount: 1,
      errorUpdateCount: 0,
      fetchFailureCount: 0,
      isInvalidated: false,
      data: [{ id: 1, title: "Ship Lens" }],
    },
    observersCount: 1,
    isActive: true,
    isStale: false,
    gcTime: 300000,
    label: "fresh",
    ...overrides,
  };
}

/** Canned answers for the browser preview; the mock never executes the typed text. */
const SAMPLE_RESULTS: Record<string, unknown> = {
  "1+1": 2,
  "1 + 1": 2,
  "2*3": 6,
  "Math.PI": Math.PI,
  "typeof window": "object",
  "2 ** 10": 1024,
  "[1,2,3].map(n => n * 2)": [2, 4, 6],
  "({ a: 1, b: 'two' })": { a: 1, b: "two" },
};

/**
 * Resolves an expression without running it: a sample from SAMPLE_RESULTS,
 * a JSON literal, or an exception for anything else (and for the special
 * "throw" expression that tests use to produce an exception entry).
 */
function evaluateStatic(expression: string): { value: unknown } | { error: string } {
  if (expression === "throw") return { error: "Error: thrown from mock" };
  if (Object.hasOwn(SAMPLE_RESULTS, expression)) return { value: SAMPLE_RESULTS[expression] };
  try {
    return { value: JSON.parse(expression) as unknown };
  } catch {
    return {
      error: `MockError: the mock backend only evaluates JSON literals and sample expressions (got \`${expression}\`)`,
    };
  }
}

/** Text Go would put in `ConsoleEntry.text` for a result value. */
function describeValue(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/** Mirrors how Go decodes a Runtime.RemoteObject for the value's type. */
function remoteObjectOf(value: unknown): RemoteObject {
  if (value === null) return { type: "object", subtype: "null", value: null, description: "null" };
  if (Array.isArray(value)) {
    return {
      type: "object",
      subtype: "array",
      className: "Array",
      description: `Array(${value.length})`,
      value,
    };
  }
  if (typeof value === "object") {
    return { type: "object", className: "Object", description: "Object", value };
  }
  return { type: typeof value, value, description: String(value) };
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
