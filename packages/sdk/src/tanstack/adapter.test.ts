import { onlineManager, QueryClient, QueryObserver } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createLensClient, type LensClient } from "../client";
import { SNAPSHOT_MAX_BYTES } from "./adapter";
import { FakeLens, waitFor } from "../testing/fake-lens";

let lens: FakeLens;
let client: LensClient;
let queryClient: QueryClient;

const defaultOptions = { queries: { retry: 0, gcTime: Infinity, staleTime: 0 } };

async function connect(): Promise<void> {
  client = createLensClient({
    url: lens.url,
    enabled: true,
    queryClient,
    onlineManager,
    backoff: { minMs: 20, maxMs: 40 },
  });
  client.connect();
  await lens.welcome();
  await lens.next("queries.snapshot");
}

beforeEach(async () => {
  lens = await FakeLens.start();
  queryClient = new QueryClient({ defaultOptions });
});

afterEach(async () => {
  client.disconnect();
  queryClient.clear();
  onlineManager.setOnline(true);
  await lens.close();
  expect(lens.invalid).toEqual([]);
});

describe("snapshot", () => {
  it("is sent right after welcome with every query and mutation", async () => {
    await queryClient.fetchQuery({
      queryKey: ["todos", { page: 1 }],
      queryFn: () => ({ items: [1, 2] }),
      meta: { tag: "list" },
    });
    const mutation = queryClient
      .getMutationCache()
      .build(queryClient, { mutationKey: ["add"], mutationFn: async (v: number) => v * 2 });
    await mutation.execute(21);

    client = createLensClient({ url: lens.url, enabled: true, queryClient, onlineManager });
    client.connect();
    await lens.welcome();
    const snapshot = await lens.next("queries.snapshot");

    expect(snapshot.payload.online).toBe(true);
    expect(snapshot.payload.queries).toHaveLength(1);
    const [query] = snapshot.payload.queries;
    expect(query).toMatchObject({
      queryHash: '["todos",{"page":1}]',
      queryKey: ["todos", { page: 1 }],
      observersCount: 0,
      isActive: false,
      isStale: false,
      gcTime: -1,
      meta: { tag: "list" },
      state: {
        status: "success",
        fetchStatus: "idle",
        dataUpdateCount: 1,
        errorUpdateCount: 0,
        fetchFailureCount: 0,
        isInvalidated: false,
        data: { items: [1, 2] },
        error: null,
        fetchMeta: null,
      },
    });
    expect(query.state.dataUpdatedAt).toBeGreaterThan(0);
    expect(query.dataTruncated).toBeUndefined();

    expect(snapshot.payload.mutations).toEqual([
      {
        mutationId: mutation.mutationId,
        mutationKey: ["add"],
        state: {
          status: "success",
          submittedAt: expect.any(Number),
          isPaused: false,
          variables: 21,
          data: 42,
          error: null,
          failureCount: 0,
          failureReason: null,
        },
      },
    ]);
  });

  it("is sent again on snapshot.request", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["a"], queryFn: () => 1 });
    lens.send({ v: 1, type: "snapshot.request", payload: {} });
    const snapshot = await lens.next("queries.snapshot");
    expect(snapshot.payload.queries.map((q) => q.queryHash)).toEqual(['["a"]']);
  });

  it("flags truncated data with the marker", async () => {
    await connect();
    const big = "x".repeat(1024 * 1024 + 10);
    await queryClient.fetchQuery({ queryKey: ["big"], queryFn: () => ({ big }) });
    lens.send({ v: 1, type: "snapshot.request", payload: {} });
    const snapshot = await lens.next("queries.snapshot");
    const [query] = snapshot.payload.queries;
    expect(query.dataTruncated).toBe(true);
    expect(query.state.data).toEqual({ __lensTruncated: true, bytes: 1024 * 1024 + 20 });
  });
  it("caps the whole snapshot at SNAPSHOT_MAX_BYTES, truncating the largest queries first", async () => {
    await connect();
    const sizes = [0, 1, 2, 3, 4, 5, 6, 7].map((i) => 1_000_000 + i * 1000);
    for (const [i, size] of sizes.entries()) {
      await queryClient.fetchQuery({ queryKey: ["big", i], queryFn: () => "x".repeat(size) });
    }
    await lens.sync();
    lens.send({ v: 1, type: "snapshot.request", payload: {} });
    const snapshot = await lens.next("queries.snapshot", 10_000);

    expect(JSON.stringify(snapshot).length).toBeLessThanOrEqual(SNAPSHOT_MAX_BYTES);
    expect(snapshot.payload.queries.map((q) => q.queryHash).sort()).toEqual(
      sizes.map((_, i) => `["big",${i}]`).sort(),
    );
    const truncated = snapshot.payload.queries.filter((q) => q.dataTruncated === true);
    expect(truncated.map((q) => q.queryHash).sort()).toEqual(['["big",6]', '["big",7]']);
    for (const q of truncated) {
      expect(q.state.data).toEqual({ __lensTruncated: true, bytes: expect.any(Number) });
    }
    for (const q of snapshot.payload.queries.filter((q) => !q.dataTruncated)) {
      expect(typeof q.state.data).toBe("string");
    }
  });
});

describe("query.event delta policy", () => {
  it("added → full query, fetch and success → deltas", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["t"], queryFn: () => "ok" });

    const added = await lens.next("query.event");
    expect(added.payload.kind).toBe("added");
    expect(added.payload.query).toMatchObject({
      queryHash: '["t"]',
      state: { status: "pending", fetchStatus: "idle" },
    });

    const fetch = await lens.next("query.event");
    expect(fetch.payload).toEqual({
      kind: "updated",
      queryHash: '["t"]',
      action: "fetch",
      delta: {
        status: "pending",
        fetchStatus: "fetching",
        fetchMeta: null,
        dataUpdatedAt: 0,
        errorUpdatedAt: 0,
      },
    });

    const success = await lens.next("query.event");
    expect(success.payload).toEqual({
      kind: "updated",
      queryHash: '["t"]',
      action: "success",
      delta: {
        status: "success",
        data: "ok",
        dataUpdatedAt: expect.any(Number),
        error: null,
        errorUpdatedAt: 0,
        fetchStatus: "idle",
      },
    });
  });

  it("error → status/error/errorUpdatedAt/fetchStatus", async () => {
    await connect();
    await queryClient
      .fetchQuery({
        queryKey: ["e"],
        queryFn: () => {
          throw new Error("nope");
        },
      })
      .catch(() => {});
    await lens.next("query.event"); // added
    await lens.next("query.event"); // fetch
    const error = await lens.next("query.event");
    expect(error.payload).toEqual({
      kind: "updated",
      queryHash: '["e"]',
      action: "error",
      delta: {
        status: "error",
        error: { name: "Error", message: "nope", stack: expect.any(String) },
        errorUpdatedAt: expect.any(Number),
        fetchStatus: "idle",
      },
    });
  });

  it("unknown action (failed) → full query", async () => {
    await connect();
    let calls = 0;
    await queryClient
      .fetchQuery({
        queryKey: ["retry"],
        retry: 1,
        retryDelay: 0,
        queryFn: () => {
          calls += 1;
          throw new Error(`fail ${calls}`);
        },
      })
      .catch(() => {});
    await lens.next("query.event"); // added
    await lens.next("query.event"); // fetch
    const failed = await lens.next("query.event");
    expect(failed.payload.action).toBe("failed");
    expect(failed.payload.delta).toBeUndefined();
    expect(failed.payload.query).toMatchObject({
      queryHash: '["retry"]',
      state: { fetchFailureCount: 1, fetchStatus: "fetching" },
    });
  });

  it("invalidate → isInvalidated", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["i"], queryFn: () => 1 });
    await lens.sync();
    lens.drain("query.event");
    await queryClient.invalidateQueries({ queryKey: ["i"], exact: true });
    const event = await lens.next("query.event");
    expect(event.payload).toEqual({
      kind: "updated",
      queryHash: '["i"]',
      action: "invalidate",
      delta: { isInvalidated: true },
    });
  });

  it("setState → the state passed, serialized", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["s"], queryFn: () => 1 });
    await lens.sync();
    lens.drain("query.event");
    queryClient
      .getQueryCache()
      .get('["s"]')!
      .setState({ data: { big: 5n }, dataUpdatedAt: 7 });
    const event = await lens.next("query.event");
    expect(event.payload).toEqual({
      kind: "updated",
      queryHash: '["s"]',
      action: "setState",
      delta: { data: { big: "5" }, dataUpdatedAt: 7 },
    });
  });

  it("setState sends an explicit null for fields the device cleared", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["c"], queryFn: () => ({ n: 1 }) });
    await lens.sync();
    lens.drain("query.event");
    queryClient
      .getQueryCache()
      .get('["c"]')!
      .setState({ data: undefined, error: undefined, status: "pending", fetchMeta: null });
    const event = await lens.next("query.event");
    expect(event.payload).toEqual({
      kind: "updated",
      queryHash: '["c"]',
      action: "setState",
      delta: { data: null, error: null, status: "pending", fetchMeta: null },
    });
  });

  it("TRIGGER_ERROR from Lens clears data on the wire", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["t"], queryFn: () => ({ n: 1 }) });
    await lens.sync();
    lens.drain("query.event");
    lens.send({
      v: 1,
      type: "query.action",
      id: "a1",
      payload: { id: "a1", type: "TRIGGER_ERROR", queryHash: '["t"]' },
    });
    await lens.next("action.result");
    const event = await lens.next("query.event");
    expect(event.payload).toMatchObject({
      kind: "updated",
      queryHash: '["t"]',
      action: "setState",
      delta: { data: null, status: "error" },
    });
    expect(event.payload.delta?.error).toMatchObject({ message: "Unknown error from devtools" });
  });

  it("pause and continue → fetchStatus", async () => {
    await connect();
    // A mounted QueryClient resumes paused fetches when onlineManager flips back.
    queryClient.mount();
    onlineManager.setOnline(false);
    await lens.next("online.status");
    const pending = queryClient
      .fetchQuery({ queryKey: ["p"], queryFn: () => "later" })
      .catch(() => {});
    await lens.next("query.event"); // added
    await lens.next("query.event"); // fetch (already paused)
    const pause = await lens.next("query.event");
    expect(pause.payload).toEqual({
      kind: "updated",
      queryHash: '["p"]',
      action: "pause",
      delta: { fetchStatus: "paused" },
    });
    onlineManager.setOnline(true);
    await lens.next("online.status");
    const cont = await lens.next("query.event");
    expect(cont.payload).toEqual({
      kind: "updated",
      queryHash: '["p"]',
      action: "continue",
      delta: { fetchStatus: "fetching" },
    });
    await pending;
    queryClient.unmount();
  });

  it("removed → hash only", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["r"], queryFn: () => 1 });
    await lens.sync();
    lens.drain("query.event");
    queryClient.removeQueries({ queryKey: ["r"], exact: true });
    const event = await lens.next("query.event");
    expect(event.payload).toEqual({ kind: "removed", queryHash: '["r"]' });
  });

  it("observer changes → kind observers; observerResultsUpdated is ignored", async () => {
    await connect();
    const observer = new QueryObserver(queryClient, { queryKey: ["o"], queryFn: () => "v" });
    const unsubscribe = observer.subscribe(() => {});
    const added = await lens.next("query.event");
    expect(added.payload.kind).toBe("added");
    const observers = await lens.next("query.event");
    expect(observers.payload).toEqual({
      kind: "observers",
      queryHash: '["o"]',
      observersCount: 1,
      isActive: true,
      isStale: true,
    });
    await waitFor(() => observer.getCurrentResult().isSuccess);
    await lens.sync();
    const kinds = lens
      .drain("query.event")
      .map((e) => (e.type === "query.event" ? e.payload : null));
    expect(kinds.map((p) => p?.action)).toEqual(["fetch", "success"]);
    unsubscribe();
    const removed = await lens.next("query.event");
    expect(removed.payload).toEqual({
      kind: "observers",
      queryHash: '["o"]',
      observersCount: 0,
      isActive: false,
      isStale: false,
    });
  });
});

describe("mutation.event", () => {
  it("added → full mutation, updated → delta, removed → id", async () => {
    await connect();
    const mutation = queryClient.getMutationCache().build(queryClient, {
      mutationKey: ["m"],
      mutationFn: async (v: { n: number }) => v.n + 1,
      gcTime: 0,
    });
    await mutation.execute({ n: 1 });

    const added = await lens.next("mutation.event");
    expect(added.payload.kind).toBe("added");
    expect(added.payload.mutation).toMatchObject({
      mutationKey: ["m"],
      state: { status: "idle", isPaused: false, submittedAt: 0 },
    });
    const id = added.payload.mutationId;

    const pending = await lens.next("mutation.event");
    expect(pending.payload).toEqual({
      kind: "updated",
      mutationId: id,
      delta: {
        status: "pending",
        submittedAt: expect.any(Number),
        isPaused: false,
        variables: { n: 1 },
        error: null,
        failureCount: 0,
        failureReason: null,
      },
    });
    let last = pending;
    while (last.payload.kind === "updated" && last.payload.delta?.status !== "success") {
      last = await lens.next("mutation.event");
    }
    expect(last.payload.delta).toMatchObject({ status: "success", data: 2, variables: { n: 1 } });

    await waitFor(() => queryClient.getMutationCache().getAll().length === 0);
    const removed = await lens.next("mutation.event");
    expect(removed.payload).toEqual({ kind: "removed", mutationId: id });
  });
});

describe("online sync", () => {
  it("reports onlineManager changes and applies online.set", async () => {
    await connect();
    onlineManager.setOnline(false);
    const status = await lens.next("online.status");
    expect(status.payload.online).toBe(false);

    lens.send({ v: 1, type: "online.set", payload: { online: true } });
    await waitFor(() => onlineManager.isOnline());
    const back = await lens.next("online.status");
    expect(back.payload.online).toBe(true);
  });
});

describe("query.action over the socket", () => {
  it("answers with action.result and 'query not found' for unknown hashes", async () => {
    await connect();
    await queryClient.fetchQuery({ queryKey: ["x"], queryFn: () => 1 });
    lens.send({
      v: 1,
      type: "query.action",
      id: "c1",
      payload: { id: "a1", type: "INVALIDATE", queryHash: '["x"]' },
    });
    const ok = await lens.next("action.result");
    expect(ok).toEqual({ v: 1, type: "action.result", id: "c1", payload: { id: "a1", ok: true } });
    await waitFor(() => queryClient.getQueryCache().get('["x"]')!.state.isInvalidated);

    lens.send({
      v: 1,
      type: "query.action",
      payload: { id: "a2", type: "REFETCH", queryHash: '["missing"]' },
    });
    const missing = await lens.next("action.result");
    expect(missing.payload).toEqual({ id: "a2", ok: false, error: "query not found" });
  });
});
