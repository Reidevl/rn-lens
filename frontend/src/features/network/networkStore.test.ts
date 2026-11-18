import { beforeEach, describe, expect, it } from "vitest";

import { createMockBackend } from "@/backend";

import { pullNetwork, useNetworkStore, visibleRows } from "./networkStore";

describe("pullNetwork", () => {
  beforeEach(() => useNetworkStore.getState().reset());

  it("merges changed rows by seq and pulls only what changed", async () => {
    const backend = createMockBackend();
    const a = backend.addRequest({ url: "https://x/a" });
    backend.addRequest({ url: "https://x/b" });
    await pullNetwork(backend);
    expect(useNetworkStore.getState().rows.map((r) => r.url)).toEqual([
      "https://x/a",
      "https://x/b",
    ]);
    const seen = useNetworkStore.getState().version;

    backend.updateRequest(a.seq, { state: "complete", status: 200 });
    await pullNetwork(backend);

    expect(backend.calls).toContain(`network.since:${seen}`);
    const rows = useNetworkStore.getState().rows;
    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({ url: "https://x/a", status: 200, state: "complete" });
  });

  it("starts over after a clear and keeps the selection only while the row exists", async () => {
    const backend = createMockBackend();
    const a = backend.addRequest({ url: "https://x/a" });
    await pullNetwork(backend);
    useNetworkStore.getState().select(a.seq);

    await backend.network.clear();
    backend.addRequest({ url: "https://x/c" });
    await pullNetwork(backend);

    const state = useNetworkStore.getState();
    expect(state.rows.map((r) => r.url)).toEqual(["https://x/c"]);
    expect(state.selectedSeq).toBeNull();
  });

  it("keeps a request that arrives between the stats and the rows", async () => {
    const backend = createMockBackend();
    const stats = backend.network.stats.bind(backend.network);
    let injected = false;
    backend.network.stats = async () => {
      const s = await stats();
      if (!injected) {
        injected = true;
        const r = backend.addRequest({ url: "https://x/fast" });
        backend.updateRequest(r.seq, { state: "complete", status: 200 });
      }
      return s;
    };
    await pullNetwork(backend);
    await pullNetwork(backend);
    expect(useNetworkStore.getState().rows.map((r) => r.url)).toEqual(["https://x/fast"]);
  });

  it("reports availability with the rows", async () => {
    const backend = createMockBackend();
    backend.setNetworkAvailability({
      checked: true,
      supported: false,
      reason: "Network inspection needs RN 0.83+",
    });
    await pullNetwork(backend);
    expect(useNetworkStore.getState().availability.reason).toBe(
      "Network inspection needs RN 0.83+",
    );
  });
});

describe("visibleRows", () => {
  const backend = createMockBackend();
  const rows = [
    backend.addRequest({ url: "https://api.x/todos", resourceType: "Fetch" }),
    backend.addRequest({ url: "https://cdn.x/logo.png", resourceType: "Image", epoch: 2 }),
  ];

  it("hides earlier sessions unless the log is preserved", () => {
    const opts = { epoch: 2, categories: [], filter: "" };
    expect(visibleRows(rows, { ...opts, preserveLog: false }).map((r) => r.url)).toEqual([
      "https://cdn.x/logo.png",
    ]);
    expect(visibleRows(rows, { ...opts, preserveLog: true })).toHaveLength(2);
  });

  it("filters by category and URL text", () => {
    const opts = { epoch: 1, preserveLog: true };
    expect(
      visibleRows(rows, { ...opts, categories: ["img"], filter: "" }).map((r) => r.url),
    ).toEqual(["https://cdn.x/logo.png"]);
    expect(
      visibleRows(rows, { ...opts, categories: [], filter: "TODOS" }).map((r) => r.url),
    ).toEqual(["https://api.x/todos"]);
  });
});
