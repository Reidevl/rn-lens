import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createLensClient, type LensClient, type LensClientState } from "./client";
import { FakeAsyncStorage, FakeMMKV } from "./storage/fakes";
import { FakeLens, waitFor } from "./testing/fake-lens";

let lens: FakeLens;
let client: LensClient | undefined;

beforeEach(async () => {
  lens = await FakeLens.start();
});

afterEach(async () => {
  client?.disconnect();
  client = undefined;
  await lens.close();
});

const fastBackoff = { minMs: 20, maxMs: 40 };

describe("createLensClient", () => {
  it("sends hello on open, waits for welcome and reports connected", async () => {
    const states: LensClientState[] = [];
    client = createLensClient({
      url: lens.url,
      enabled: true,
      appName: "Example",
      appId: "com.example",
      deviceName: "Test device",
      token: "secret",
      backoff: fastBackoff,
    });
    client.onStateChange((s) => states.push(s));
    expect(client.state).toBe("idle");
    client.connect();
    expect(client.state).toBe("connecting");

    const hello = await lens.welcome();
    expect(hello.payload).toMatchObject({
      sdkVersion: expect.any(String),
      protocolVersion: 1,
      app: { id: "com.example", name: "Example", platform: "other" },
      device: { id: expect.any(String), name: "Test device" },
      features: [],
      token: "secret",
    });
    await waitFor(() => client?.state === "connected");
    expect(states).toEqual(["connecting", "connected"]);
    expect(lens.invalid).toEqual([]);
  });

  it("lists the queries feature only with a queryClient", async () => {
    const { QueryClient } = await import("@tanstack/react-query");
    client = createLensClient({
      url: lens.url,
      enabled: true,
      queryClient: new QueryClient(),
      backoff: fastBackoff,
    });
    client.connect();
    const hello = await lens.next("hello");
    expect(hello.payload.features).toEqual(["queries"]);
  });

  it("lists the storage feature only with storage options", async () => {
    client = createLensClient({
      url: lens.url,
      enabled: true,
      storage: { asyncStorage: new FakeAsyncStorage({ a: "1" }) },
      backoff: fastBackoff,
    });
    client.connect();
    const hello = await lens.welcome();
    expect(hello.payload.features).toEqual(["storage"]);
    const snapshot = await lens.next("storage.snapshot");
    expect(snapshot.payload.stores.map((s) => s.id)).toEqual(["async-storage"]);
    await lens.sync();
    expect(lens.drain("queries.snapshot")).toEqual([]);
    expect(lens.invalid).toEqual([]);
  });

  it("lists both features and sends both snapshots with a queryClient and storage", async () => {
    const { QueryClient } = await import("@tanstack/react-query");
    client = createLensClient({
      url: lens.url,
      enabled: true,
      queryClient: new QueryClient(),
      storage: { mmkv: { default: new FakeMMKV({ k: "v" }) } },
      backoff: fastBackoff,
    });
    client.connect();
    const hello = await lens.welcome();
    expect(hello.payload.features).toEqual(["queries", "storage"]);
    await lens.next("queries.snapshot");
    await lens.next("storage.snapshot");
    // Inbound frames reach both adapters.
    lens.send({ v: 1, type: "snapshot.request", payload: {} });
    await lens.next("queries.snapshot");
    lens.send({
      v: 1,
      type: "storage.action",
      id: "s1",
      payload: { id: "s1", type: "GET", storeId: "mmkv:default", key: "k" },
    });
    const result = await lens.next("storage.result");
    expect(result.payload).toEqual({
      id: "s1",
      ok: true,
      value: { type: "string", value: "v", size: 1 },
    });
    expect(lens.invalid).toEqual([]);
  });

  it("answers ping with pong carrying the same id", async () => {
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.connect();
    await lens.welcome();
    lens.send({ v: 1, type: "ping", id: "p1", payload: {} });
    const pong = await lens.next("pong");
    expect(pong).toEqual({ v: 1, type: "pong", id: "p1", payload: {} });
  });

  it("ignores frames that are not Lens messages", async () => {
    const warn = vi.fn();
    client = createLensClient({
      url: lens.url,
      enabled: true,
      backoff: fastBackoff,
      logger: { warn },
    });
    client.connect();
    await lens.welcome();
    lens.sendRaw("not json");
    lens.sendRaw(JSON.stringify({ v: 2, type: "ping", payload: {} }));
    lens.send({ v: 1, type: "ping", payload: {} });
    await lens.next("pong");
    expect(warn).toHaveBeenCalledTimes(2);
    expect(client.state).toBe("connected");
  });

  it("accepts frames carrying fields this SDK does not know (additive minors)", async () => {
    const { QueryClient } = await import("@tanstack/react-query");
    const warn = vi.fn();
    client = createLensClient({
      url: lens.url,
      enabled: true,
      queryClient: new QueryClient(),
      backoff: fastBackoff,
      logger: { warn },
    });
    client.connect();
    await lens.next("hello");
    lens.sendRaw(
      JSON.stringify({
        v: 1,
        type: "welcome",
        future: true,
        payload: { lensVersion: "9.9.0", protocolVersion: 1, sessionId: "s1", motd: "hi" },
      }),
    );
    await waitFor(() => client?.state === "connected");
    await lens.next("queries.snapshot");

    lens.sendRaw(
      JSON.stringify({
        v: 1,
        type: "query.action",
        id: "a1",
        payload: { id: "a1", type: "REFETCH", queryHash: '["nope"]', priority: "high" },
      }),
    );
    const result = await lens.next("action.result");
    expect(result.payload).toEqual({ id: "a1", ok: false, error: "query not found" });
    expect(warn).not.toHaveBeenCalled();
    expect(client.state).toBe("connected");
  });

  it("reconnects with backoff after the server closes the socket", async () => {
    const states: LensClientState[] = [];
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.onStateChange((s) => states.push(s));
    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");

    lens.closeClients();
    await waitFor(() => client?.state === "reconnecting");
    const hello = await lens.welcome();
    expect(hello.type).toBe("hello");
    await waitFor(() => client?.state === "connected");
    expect(states).toEqual(["connecting", "connected", "reconnecting", "connected"]);
  });

  it.each([
    [4002, "protocol version"],
    [4003, "token"],
  ])("stops reconnecting after fatal close code %i and warns", async (code, meaning) => {
    const warn = vi.fn();
    client = createLensClient({
      url: lens.url,
      enabled: true,
      backoff: fastBackoff,
      logger: { warn },
    });
    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");

    lens.closeClients(code);
    await waitFor(() => client?.state === "disabled");
    expect(warn).toHaveBeenCalledTimes(1);
    const [message] = warn.mock.calls[0] as [string];
    expect(message).toContain(String(code));
    expect(message).toContain(meaning);

    await new Promise((r) => setTimeout(r, 150));
    expect(lens.connectionCount).toBe(0);
    expect(lens.frames.filter((f) => f.type === "hello")).toHaveLength(1);
    expect(client.state).toBe("disabled");
  });

  it("goes disabled with one warning after three consecutive 4001 closes", async () => {
    const warn = vi.fn();
    const states: LensClientState[] = [];
    client = createLensClient({
      url: lens.url,
      enabled: true,
      backoff: fastBackoff,
      logger: { warn },
    });
    client.onStateChange((s) => states.push(s));
    client.connect();
    for (let i = 0; i < 3; i++) {
      await lens.next("hello");
      lens.closeClients(4001);
    }
    await waitFor(() => client?.state === "disabled");
    expect(warn).toHaveBeenCalledTimes(1);
    const [message] = warn.mock.calls[0] as [string];
    expect(message).toContain("4001");
    expect(message).toContain("hello");

    await new Promise((r) => setTimeout(r, 150));
    expect(lens.connectionCount).toBe(0);
    expect(lens.frames.filter((f) => f.type === "hello")).toHaveLength(3);
    expect(states).toEqual(["connecting", "reconnecting", "disabled"]);

    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
  });

  it("a welcome resets the 4001 counter", async () => {
    const warn = vi.fn();
    client = createLensClient({
      url: lens.url,
      enabled: true,
      backoff: fastBackoff,
      logger: { warn },
    });
    client.connect();
    for (let i = 0; i < 2; i++) {
      await lens.next("hello");
      lens.closeClients(4001);
    }
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
    lens.closeClients(4001);
    await lens.next("hello");
    lens.closeClients(4001);
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
    expect(warn).not.toHaveBeenCalled();
    expect(lens.frames.filter((f) => f.type === "hello")).toHaveLength(5);
  });

  it("keeps reconnecting after every other close code", async () => {
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
    lens.closeClients(4004);
    await waitFor(() => client?.state === "reconnecting");
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
  });

  it("keeps retrying while the server is down and connects once it is back", async () => {
    const port = Number(new URL(lens.url).port);
    await lens.close();
    client = createLensClient({
      url: `ws://127.0.0.1:${port}/lens`,
      enabled: true,
      backoff: fastBackoff,
    });
    client.connect();
    await new Promise((r) => setTimeout(r, 120));
    expect(client.state).toBe("reconnecting");

    lens = await FakeLens.start();
    client.disconnect();
    expect(client.state).toBe("idle");
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
  });

  it("disconnect closes the socket, stops reconnecting and goes idle", async () => {
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.connect();
    await lens.welcome();
    await waitFor(() => lens.connectionCount === 1);
    client.disconnect();
    expect(client.state).toBe("idle");
    await waitFor(() => lens.connectionCount === 0);
    await new Promise((r) => setTimeout(r, 100));
    expect(lens.connectionCount).toBe(0);
    expect(client.send({ v: 1, type: "pong", payload: {} })).toBe(false);
  });

  it("never connects when disabled", async () => {
    client = createLensClient({ url: lens.url, enabled: false });
    expect(client.state).toBe("disabled");
    client.connect();
    await new Promise((r) => setTimeout(r, 100));
    expect(lens.connectionCount).toBe(0);
    expect(client.send({ v: 1, type: "pong", payload: {} })).toBe(false);
    expect(client.registerPlugin({ id: "x" })).toBeTypeOf("function");
    expect(client.onStateChange(() => {})).toBeTypeOf("function");
    client.disconnect();
    expect(client.state).toBe("disabled");
  });

  it("uses the injected WebSocket implementation", async () => {
    const { WebSocket: WsClient } = await import("ws");
    client = createLensClient({
      url: lens.url,
      enabled: true,
      backoff: fastBackoff,
      WebSocketImpl: WsClient as unknown as NonNullable<
        Parameters<typeof createLensClient>[0]
      >["WebSocketImpl"],
    });
    client.connect();
    await lens.welcome();
    await waitFor(() => client?.state === "connected");
  });

  it("delivers every inbound frame to registered plugins", async () => {
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    const seen: string[] = [];
    const unregister = client.registerPlugin({ id: "spy", onMessage: (m) => seen.push(m.type) });
    client.connect();
    await lens.welcome();
    lens.send({ v: 1, type: "ping", payload: {} });
    await lens.next("pong");
    expect(seen).toEqual(["welcome", "ping"]);
    unregister();
    lens.send({ v: 1, type: "ping", payload: {} });
    await lens.next("pong");
    expect(seen).toEqual(["welcome", "ping"]);
  });

  it("sends arbitrary frames through send()", async () => {
    client = createLensClient({ url: lens.url, enabled: true, backoff: fastBackoff });
    client.connect();
    await lens.welcome();
    expect(client.send({ v: 1, type: "online.status", payload: { online: false } })).toBe(true);
    const status = await lens.next("online.status");
    expect(status.payload.online).toBe(false);
  });
});
