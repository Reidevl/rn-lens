import type { ActionResultPayload, QueryActionPayload } from "@rnlens/protocol";
import type { Query, QueryClient, QueryOptions, QueryState } from "@tanstack/react-query";

const noop = (): void => {};

type FetchMeta = NonNullable<QueryState["fetchMeta"]>;

/** `fetchMeta` shape the official devtools use to remember the options a TRIGGER_* replaced. */
export type DevtoolsFetchMeta = FetchMeta & { __previousQueryOptions?: QueryOptions };

function previousOptions(query: Query): QueryOptions | undefined {
  const meta = query.state.fetchMeta as DevtoolsFetchMeta | null;
  return meta?.__previousQueryOptions;
}

function triggerLoading(query: Query): void {
  const __previousQueryOptions = query.options;
  void query
    .fetch({ ...query.options, queryFn: () => new Promise<never>(() => {}), gcTime: -1 })
    .catch(noop);
  const fetchMeta: DevtoolsFetchMeta = { ...query.state.fetchMeta, __previousQueryOptions };
  query.setState({ data: undefined, status: "pending", fetchMeta });
}

function restoreLoading(query: Query): void {
  const previous = previousOptions(query);
  void query.cancel({ silent: true }).catch(noop);
  query.setState({ ...query.state, fetchStatus: "idle", fetchMeta: null });
  if (previous) void query.fetch(previous).catch(noop);
}

function triggerError(query: Query): void {
  const fetchMeta: DevtoolsFetchMeta = {
    ...query.state.fetchMeta,
    __previousQueryOptions: query.options,
  };
  query.setState({
    data: undefined,
    status: "error",
    error: new Error("Unknown error from devtools"),
    fetchMeta,
  });
}

/**
 * Executes one `query.action` on the device with the recipes of the official
 * TanStack Query devtools and returns the `action.result` payload.
 */
export async function runQueryAction(
  queryClient: QueryClient,
  action: QueryActionPayload,
): Promise<ActionResultPayload> {
  const ok: ActionResultPayload = { id: action.id, ok: true };
  try {
    if (action.type === "CLEAR_QUERY_CACHE") {
      queryClient.getQueryCache().clear();
      return ok;
    }
    if (action.type === "CLEAR_MUTATION_CACHE") {
      queryClient.getMutationCache().clear();
      return ok;
    }
    const query =
      action.queryHash === undefined
        ? undefined
        : queryClient.getQueryCache().get(action.queryHash);
    if (!query) return { id: action.id, ok: false, error: "query not found" };
    const exact = { queryKey: query.queryKey, exact: true as const };

    switch (action.type) {
      case "REFETCH":
        void query.fetch().catch(noop);
        return ok;
      case "INVALIDATE":
        void queryClient.invalidateQueries(exact).catch(noop);
        return ok;
      case "RESET":
        void queryClient.resetQueries(exact).catch(noop);
        return ok;
      case "REMOVE":
        queryClient.removeQueries(exact);
        return ok;
      case "SET_QUERY_DATA":
        // Same call as the official devtools explorer: observers are notified
        // and dataUpdatedAt / dataUpdateCount advance like after a fetch.
        if (action.data === undefined) {
          return { id: action.id, ok: false, error: "SET_QUERY_DATA requires data" };
        }
        queryClient.setQueryData(query.queryKey, action.data);
        return ok;
      case "TRIGGER_LOADING":
        triggerLoading(query);
        return ok;
      case "RESTORE_LOADING":
        restoreLoading(query);
        return ok;
      case "TRIGGER_ERROR":
        triggerError(query);
        return ok;
      case "RESTORE_ERROR":
        void queryClient.resetQueries(exact).catch(noop);
        return ok;
      default:
        return { id: action.id, ok: false, error: `unknown action ${String(action.type)}` };
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return { id: action.id, ok: false, error: message };
  }
}
