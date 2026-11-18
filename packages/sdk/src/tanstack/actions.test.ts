import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { waitFor } from "../testing/fake-lens";
import { runQueryAction, type DevtoolsFetchMeta } from "./actions";

let queryClient: QueryClient;
let calls: number;
const key = ["todos"];
const hash = '["todos"]';

beforeEach(async () => {
  calls = 0;
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: 0, gcTime: Infinity, staleTime: Infinity } },
  });
  await queryClient.fetchQuery({
    queryKey: key,
    queryFn: async () => {
      calls += 1;
      return { n: calls };
    },
  });
});

afterEach(() => {
  queryClient.clear();
});

const query = () => queryClient.getQueryCache().get(hash)!;
const run = (type: Parameters<typeof runQueryAction>[1]["type"], extra = {}) =>
  runQueryAction(queryClient, { id: "a", type, queryHash: hash, ...extra });

describe("runQueryAction", () => {
  it("REFETCH runs the queryFn again", async () => {
    expect(await run("REFETCH")).toEqual({ id: "a", ok: true });
    await waitFor(() => calls === 2);
    await waitFor(() => query().state.fetchStatus === "idle");
    expect(query().state.data).toEqual({ n: 2 });
  });

  it("INVALIDATE marks the query invalidated and refetches active observers", async () => {
    const observer = new QueryObserver(queryClient, { queryKey: key, queryFn: () => ({ n: 99 }) });
    const unsubscribe = observer.subscribe(() => {});
    expect(await run("INVALIDATE")).toEqual({ id: "a", ok: true });
    expect(query().state.isInvalidated).toBe(true);
    await waitFor(() => (query().state.data as { n: number } | undefined)?.n === 99);
    unsubscribe();
  });

  it("RESET returns the query to its initial state", async () => {
    expect(await run("RESET")).toEqual({ id: "a", ok: true });
    expect(query().state).toMatchObject({ status: "pending", data: undefined, dataUpdateCount: 0 });
  });

  it("REMOVE drops the query from the cache", async () => {
    expect(await run("REMOVE")).toEqual({ id: "a", ok: true });
    expect(queryClient.getQueryCache().get(hash)).toBeUndefined();
  });

  // Official recipe (@tanstack/query-devtools Explorer.tsx): queryClient.setQueryData(queryKey, data),
  // so observers are notified and dataUpdatedAt / dataUpdateCount advance like after a fetch.
  it("SET_QUERY_DATA goes through queryClient.setQueryData: observers see it and dataUpdatedAt advances", async () => {
    const observer = new QueryObserver(queryClient, { queryKey: key, queryFn: () => ({ n: 99 }) });
    const unsubscribe = observer.subscribe(() => {});
    const before = query().state;
    await new Promise((r) => setTimeout(r, 5));
    expect(await run("SET_QUERY_DATA", { data: { n: 7, extra: true } })).toEqual({
      id: "a",
      ok: true,
    });
    expect(query().state.data).toEqual({ n: 7, extra: true });
    expect(query().state.status).toBe("success");
    expect(query().state.dataUpdatedAt).toBeGreaterThan(before.dataUpdatedAt);
    expect(query().state.dataUpdateCount).toBe(before.dataUpdateCount + 1);
    expect(observer.getCurrentResult().data).toEqual({ n: 7, extra: true });
    expect(calls).toBe(1);
    unsubscribe();
  });

  it("SET_QUERY_DATA without data fails instead of clearing the query", async () => {
    expect(await run("SET_QUERY_DATA")).toEqual({
      id: "a",
      ok: false,
      error: "SET_QUERY_DATA requires data",
    });
    expect(query().state.data).toEqual({ n: 1 });
  });

  it("TRIGGER_LOADING then RESTORE_LOADING", async () => {
    const before = query().options;
    expect(await run("TRIGGER_LOADING")).toEqual({ id: "a", ok: true });
    expect(query().state).toMatchObject({
      status: "pending",
      data: undefined,
      fetchStatus: "fetching",
    });
    const meta = query().state.fetchMeta as DevtoolsFetchMeta;
    expect(meta.__previousQueryOptions).toBe(before);
    await new Promise((r) => setTimeout(r, 20));
    expect(query().state.fetchStatus).toBe("fetching");

    expect(await run("RESTORE_LOADING")).toEqual({ id: "a", ok: true });
    await waitFor(() => query().state.status === "success" && query().state.fetchStatus === "idle");
    expect(query().state.data).toEqual({ n: 2 });
    expect(query().state.fetchMeta).toBeNull();
  });

  // Official recipe (@tanstack/query-devtools Devtools.tsx, triggerError):
  // setState({ data: undefined, status: 'error', error, fetchMeta: { ...fetchMeta, __previousQueryOptions } }).
  it("TRIGGER_ERROR then RESTORE_ERROR", async () => {
    const observer = new QueryObserver(queryClient, { queryKey: key, queryFn: () => ({ n: 5 }) });
    const unsubscribe = observer.subscribe(() => {});
    query().setState({ fetchMeta: { tag: "kept" } as DevtoolsFetchMeta });
    const before = query().options;
    expect(await run("TRIGGER_ERROR")).toEqual({ id: "a", ok: true });
    expect(query().state).toMatchObject({ status: "error", fetchStatus: "idle" });
    expect("data" in query().state && query().state.data === undefined).toBe(true);
    expect((query().state.error as Error).message).toBe("Unknown error from devtools");
    const meta = query().state.fetchMeta as DevtoolsFetchMeta & { tag?: string };
    expect(meta.__previousQueryOptions).toBe(before);
    expect(meta.tag).toBe("kept");

    expect(await run("RESTORE_ERROR")).toEqual({ id: "a", ok: true });
    await waitFor(() => query().state.status === "success");
    expect(query().state.error).toBeNull();
    expect(query().state.data).toEqual({ n: 5 });
    unsubscribe();
  });

  it("CLEAR_QUERY_CACHE and CLEAR_MUTATION_CACHE empty the caches", async () => {
    const mutation = queryClient
      .getMutationCache()
      .build(queryClient, { mutationFn: async () => 1 });
    await mutation.execute(undefined);
    expect(queryClient.getMutationCache().getAll()).toHaveLength(1);

    expect(await runQueryAction(queryClient, { id: "q", type: "CLEAR_QUERY_CACHE" })).toEqual({
      id: "q",
      ok: true,
    });
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
    expect(await runQueryAction(queryClient, { id: "m", type: "CLEAR_MUTATION_CACHE" })).toEqual({
      id: "m",
      ok: true,
    });
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
  });

  it("reports unknown hashes and thrown errors", async () => {
    expect(
      await runQueryAction(queryClient, { id: "u", type: "REFETCH", queryHash: "nope" }),
    ).toEqual({
      id: "u",
      ok: false,
      error: "query not found",
    });
    expect(await runQueryAction(queryClient, { id: "u2", type: "REFETCH" })).toEqual({
      id: "u2",
      ok: false,
      error: "query not found",
    });
    const broken = {
      getQueryCache: () => {
        throw new Error("cache exploded");
      },
    } as unknown as QueryClient;
    expect(await runQueryAction(broken, { id: "b", type: "CLEAR_QUERY_CACHE" })).toEqual({
      id: "b",
      ok: false,
      error: "cache exploded",
    });
  });
});
