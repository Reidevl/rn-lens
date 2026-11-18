import {
  isLensMessage,
  PROTOCOL_VERSION,
  type HelloMessage,
  type HelloPayload,
  type LensMessage,
} from "@rnlens/protocol";
import type { QueryClient } from "@tanstack/react-query";

import { getDeviceId } from "./device-id";
import { defaultEnabled, readRuntimeEnv, resolveLensUrl } from "./env";
import { makeLog, type LensLogger, type Log } from "./logger";
import { DEFAULT_MAX_BYTES, DEFAULT_MAX_DEPTH } from "./serialize";
import { createQueryAdapter, type OnlineManagerLike, type QueryAdapter } from "./tanstack/adapter";
import { SDK_VERSION } from "./version";

/**
 * `disabled` is the state of a production (no-op) client and of a client that
 * stopped after a fatal close code (4002, 4003, or 4001 three times in a row);
 * `connect()` retries the latter.
 */
export type LensClientState = "idle" | "connecting" | "connected" | "reconnecting" | "disabled";

/** Minimal WebSocket surface the client needs; the global WebSocket satisfies it. */
export interface LensWebSocket {
  readyState: number;
  onopen: ((event?: unknown) => void) | null;
  onclose: ((event: { code?: number; reason?: string }) => void) | null;
  onerror: ((event?: unknown) => void) | null;
  onmessage: ((event: { data: unknown }) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export type WebSocketConstructor = new (url: string) => LensWebSocket;

export interface LensPlugin {
  id: string;
  /** Receives every inbound frame, after the SDK handled it. */
  onMessage?(message: LensMessage): void;
}

export interface LensClientOptions {
  /** TanStack `QueryClient` to mirror; without it the Queries feature is off. */
  queryClient?: QueryClient;
  /** Full WebSocket URL; derived from the Metro bundle host when omitted. */
  url?: string;
  /** Pairing token when Lens runs with `--sdk-token`. */
  token?: string;
  /** Shown in Lens's device list; defaults to the device model. */
  deviceName?: string;
  /** Shown in Lens; defaults to `appId`. */
  appName?: string;
  /** Bundle id or package name; defaults to `appName` or "app". */
  appId?: string;
  /** Defaults to `__DEV__`; when false every method is a no-op. */
  enabled?: boolean;
  /** Pass `console` to see connection diagnostics. */
  logger?: LensLogger;
  /** WebSocket implementation; defaults to the global `WebSocket`. */
  WebSocketImpl?: WebSocketConstructor;
  /** Reconnect backoff bounds in milliseconds (default 500 → 10 000). */
  backoff?: { minMs?: number; maxMs?: number };
  /** Test hook: replaces the `onlineManager` the adapter syncs. */
  onlineManager?: OnlineManagerLike;
}

export interface LensClient {
  readonly state: LensClientState;
  connect(): void;
  disconnect(): void;
  onStateChange(listener: (state: LensClientState) => void): () => void;
  /** Sends a frame when the socket is open; returns false otherwise. */
  send(message: LensMessage): boolean;
  /** Reserved for M5+: observes every inbound frame. Returns an unregister function. */
  registerPlugin(plugin: LensPlugin): () => void;
}

const WS_OPEN = 1;
const DEFAULT_MIN_BACKOFF = 500;
const DEFAULT_MAX_BACKOFF = 10_000;

/**
 * Close codes that no reconnect can fix: the client stops, warns and goes
 * `disabled` until `connect()` is called again (protocol/README.md).
 */
const FATAL_CLOSE_CODES: Readonly<Record<number, string>> = {
  4002: "Lens speaks another protocol version; update @rnlens/sdk or Lens",
  4003: "pairing token missing or wrong; pass the token Lens shows",
};

/** Lens closes with 4001 when no `hello` arrived within its handshake timeout. */
const HELLO_TIMEOUT_CLOSE_CODE = 4001;
/**
 * Consecutive 4001 closes (no `welcome` in between) after which the client
 * stops like after 4002/4003 instead of looping hello → timeout → reconnect.
 */
const MAX_CONSECUTIVE_HELLO_TIMEOUTS = 3;

const NOOP_CLIENT: LensClient = {
  state: "disabled",
  connect: () => {},
  disconnect: () => {},
  onStateChange: () => () => {},
  send: () => false,
  registerPlugin: () => () => {},
};

function backoffDelay(attempt: number, minMs: number, maxMs: number): number {
  const base = Math.min(maxMs, minMs * 2 ** attempt);
  const jitter = base * (0.8 + Math.random() * 0.4);
  return Math.round(Math.min(maxMs, jitter));
}

function parseFrame(data: unknown, log: Log): LensMessage | undefined {
  if (typeof data !== "string") {
    log.warn("ignoring non-text frame");
    return undefined;
  }
  try {
    const parsed: unknown = JSON.parse(data);
    if (isLensMessage(parsed)) return parsed;
    log.warn("ignoring frame that is not a Lens message");
  } catch {
    log.warn("ignoring frame that is not JSON");
  }
  return undefined;
}

/**
 * Creates the Lens client. When `enabled` is false (production bundles) the
 * returned client never opens a socket and every method is a no-op.
 */
export function createLensClient(options: LensClientOptions = {}): LensClient {
  const enabled = options.enabled ?? defaultEnabled();
  if (!enabled) return NOOP_CLIENT;

  const log = makeLog(options.logger);
  const minMs = options.backoff?.minMs ?? DEFAULT_MIN_BACKOFF;
  const maxMs = options.backoff?.maxMs ?? DEFAULT_MAX_BACKOFF;
  const limits = { maxBytes: DEFAULT_MAX_BYTES, maxDepth: DEFAULT_MAX_DEPTH };
  const listeners = new Set<(state: LensClientState) => void>();
  const plugins = new Map<string, LensPlugin>();

  let state: LensClientState = "idle";
  let socket: LensWebSocket | undefined;
  let ready = false;
  let attempt = 0;
  let helloTimeouts = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  let closedByUser = true;

  function setState(next: LensClientState): void {
    if (state === next) return;
    state = next;
    for (const listener of listeners) listener(next);
  }

  function sendRaw(json: string): boolean {
    if (!socket || socket.readyState !== WS_OPEN) return false;
    try {
      socket.send(json);
      return true;
    } catch (err) {
      log.warn("send failed", err);
      return false;
    }
  }

  function send(message: LensMessage): boolean {
    return sendRaw(JSON.stringify(message));
  }

  const adapter: QueryAdapter | undefined = options.queryClient
    ? createQueryAdapter(
        options.queryClient,
        { send, sendRaw, isReady: () => ready, log },
        { limits, onlineManager: options.onlineManager },
      )
    : undefined;

  async function buildHello(): Promise<HelloMessage> {
    const env = readRuntimeEnv();
    const deviceId = await getDeviceId();
    const appId = options.appId ?? options.appName ?? "app";
    const payload: HelloPayload = {
      sdkVersion: SDK_VERSION,
      protocolVersion: PROTOCOL_VERSION,
      app: { id: appId, platform: env.platform },
      device: { id: deviceId },
      features: adapter ? ["queries"] : [],
    };
    const appName = options.appName ?? options.appId;
    if (appName) payload.app.name = appName;
    if (env.rnVersion) payload.app.rnVersion = env.rnVersion;
    const deviceName = options.deviceName ?? env.deviceModel;
    if (deviceName) payload.device.name = deviceName;
    if (env.scriptURL) payload.scriptURL = env.scriptURL;
    if (options.token) payload.token = options.token;
    return { v: 1, type: "hello", payload };
  }

  function resolveUrl(): string {
    return options.url ?? resolveLensUrl(readRuntimeEnv());
  }

  function handleMessage(message: LensMessage): void {
    switch (message.type) {
      case "welcome":
        ready = true;
        attempt = 0;
        helloTimeouts = 0;
        setState("connected");
        log.info(`connected to Lens ${message.payload.lensVersion}`);
        adapter?.sendSnapshot();
        break;
      case "ping":
        send({ v: 1, type: "pong", id: message.id, payload: {} });
        break;
      default:
        if (!adapter?.handle(message)) log.debug(`unhandled message ${message.type}`);
    }
    for (const plugin of plugins.values()) {
      try {
        plugin.onMessage?.(message);
      } catch (err) {
        log.warn(`plugin ${plugin.id} threw`, err);
      }
    }
  }

  function scheduleReconnect(): void {
    if (closedByUser || reconnectTimer) return;
    const delay = backoffDelay(attempt, minMs, maxMs);
    attempt += 1;
    setState("reconnecting");
    log.debug(`reconnecting in ${delay} ms`);
    reconnectTimer = setTimeout(() => {
      reconnectTimer = undefined;
      open();
    }, delay);
  }

  function teardownSocket(): void {
    ready = false;
    if (!socket) return;
    const current = socket;
    socket = undefined;
    current.onopen = null;
    current.onmessage = null;
    current.onerror = null;
    current.onclose = null;
    try {
      current.close();
    } catch {
      // already closed
    }
  }

  function open(): void {
    if (closedByUser || socket) return;
    const Impl =
      options.WebSocketImpl ?? (globalThis as { WebSocket?: WebSocketConstructor }).WebSocket;
    if (!Impl) {
      log.error("no WebSocket implementation available; Lens disabled");
      setState("idle");
      closedByUser = true;
      return;
    }
    const url = resolveUrl();
    if (attempt === 0) setState("connecting");
    let ws: LensWebSocket;
    try {
      ws = new Impl(url);
    } catch (err) {
      log.warn(`cannot open ${url}`, err);
      scheduleReconnect();
      return;
    }
    socket = ws;
    ws.onopen = () => {
      void buildHello().then((hello) => {
        if (socket === ws) send(hello);
      });
    };
    ws.onmessage = (event) => {
      const message = parseFrame(event.data, log);
      if (message) handleMessage(message);
    };
    const handleClosed = (code: number | undefined): void => {
      if (socket !== ws) return;
      socket = undefined;
      ready = false;
      let fatal = code === undefined ? undefined : FATAL_CLOSE_CODES[code];
      if (code === HELLO_TIMEOUT_CLOSE_CODE) {
        helloTimeouts += 1;
        if (helloTimeouts >= MAX_CONSECUTIVE_HELLO_TIMEOUTS) {
          fatal =
            `no hello reached Lens within its handshake timeout ${helloTimeouts} times in a row; ` +
            "check the url and that the app can reach Lens";
        }
      }
      if (fatal !== undefined) {
        log.warn(`Lens closed the connection with code ${code}: ${fatal}; not reconnecting`);
        closedByUser = true;
        adapter?.detach();
        setState("disabled");
        return;
      }
      log.debug(`socket closed (${code ?? "?"})`);
      scheduleReconnect();
    };
    ws.onerror = () => {
      log.debug(`socket error on ${url}`);
      // Some implementations (Node's WebSocket among them) fire only `error`
      // when the connection is refused and never `close`; a socket that
      // errored before opening is treated as closed.
      if (ws.readyState !== WS_OPEN) {
        handleClosed(undefined);
        try {
          ws.close();
        } catch {
          // never opened
        }
      }
    };
    ws.onclose = (event) => handleClosed(event.code);
  }

  function connect(): void {
    if (!closedByUser) return;
    closedByUser = false;
    attempt = 0;
    helloTimeouts = 0;
    adapter?.attach();
    open();
  }

  function disconnect(): void {
    closedByUser = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
    }
    adapter?.detach();
    teardownSocket();
    attempt = 0;
    setState("idle");
  }

  return {
    get state() {
      return state;
    },
    connect,
    disconnect,
    onStateChange(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    send,
    registerPlugin(plugin) {
      plugins.set(plugin.id, plugin);
      return () => {
        plugins.delete(plugin.id);
      };
    },
  };
}
