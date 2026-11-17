import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createLensClient, type LensClient, type LensClientState } from "./client";
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
