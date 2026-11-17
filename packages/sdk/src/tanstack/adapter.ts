import type {
  LensMessage,
  MutationEventMessage,
  QueriesSnapshotMessage,
  QueryEventMessage,
} from "@rnlens/protocol";
import type {
  MutationCacheNotifyEvent,
  QueryCacheNotifyEvent,
  QueryClient,
  onlineManager as OnlineManager,
} from "@tanstack/react-query";

import type { Log } from "../logger";
import { loadReactQuery } from "../optional-require.cjs";
import { runQueryAction } from "./actions";
import {
  mutationDelta,
  pickQueryState,
  QUERY_ACTION_DELTA_FIELDS,
  QUERY_DELTA_KEYS,
  serializeMutation,
  serializeQuery,
  type SerializeLimits,
} from "./serialize";

export type OnlineManagerLike = Pick<typeof OnlineManager, "subscribe" | "setOnline" | "isOnline">;

/** The piece of the client the adapter talks to. */
export interface AdapterHost {
  send(message: LensMessage): boolean;
  /** True once `welcome` arrived on the current socket. */
  isReady(): boolean;
  log: Log;
}

export interface QueryAdapter {
  attach(): void;
  detach(): void;
  sendSnapshot(): void;
  /** Handles an inbound frame; returns true when the adapter consumed it. */
  handle(message: LensMessage): boolean;
}

export interface QueryAdapterOptions {
  limits: SerializeLimits;
  /** Injected for tests; defaults to `onlineManager` from `@tanstack/react-query`. */
  onlineManager?: OnlineManagerLike;
}

function resolveOnlineManager(log: Log): OnlineManagerLike | undefined {
  const mod = loadReactQuery<{ onlineManager?: OnlineManagerLike }>();
  if (mod?.onlineManager) return mod.onlineManager;
  log.warn("@tanstack/react-query not resolvable; online sync disabled");
  return undefined;
}

export function createQueryAdapter(
  queryClient: QueryClient,
  host: AdapterHost,
  options: QueryAdapterOptions,
): QueryAdapter {
  const { limits } = options;
  let onlineManager = options.onlineManager;
  let unsubscribers: Array<() => void> = [];

  const queryEvent = (payload: QueryEventMessage["payload"]): QueryEventMessage => ({
    v: 1,
    type: "query.event",
    payload,
  });
  const mutationEvent = (payload: MutationEventMessage["payload"]): MutationEventMessage => ({
    v: 1,
    type: "mutation.event",
    payload,
  });

  function onQueryEvent(event: QueryCacheNotifyEvent): void {
    if (!host.isReady()) return;
    const query = event.query;
    const queryHash = query.queryHash;
    switch (event.type) {
      case "added":
        host.send(queryEvent({ kind: "added", queryHash, query: serializeQuery(query, limits) }));
        return;
      case "removed":
        host.send(queryEvent({ kind: "removed", queryHash }));
        return;
      case "updated": {
        const action = event.action.type;
        if (action === "setState") {
          const { delta } = pickQueryState(event.action.state, QUERY_DELTA_KEYS, limits);
          host.send(queryEvent({ kind: "updated", queryHash, action, delta }));
          return;
        }
        const fields = QUERY_ACTION_DELTA_FIELDS[action];
        if (fields) {
          const { delta } = pickQueryState(query.state, fields, limits);
          host.send(queryEvent({ kind: "updated", queryHash, action, delta }));
          return;
        }
        host.send(
          queryEvent({ kind: "updated", queryHash, action, query: serializeQuery(query, limits) }),
        );
        return;
      }
      case "observerAdded":
      case "observerRemoved":
      case "observerOptionsUpdated":
        host.send(
          queryEvent({
            kind: "observers",
            queryHash,
            observersCount: query.getObserversCount(),
            isActive: query.isActive(),
            isStale: query.isStale(),
          }),
        );
        return;
      default:
        return;
    }
  }

  function onMutationEvent(event: MutationCacheNotifyEvent): void {
    if (!host.isReady()) return;
    const mutation = event.mutation;
    if (!mutation) return;
    const mutationId = mutation.mutationId;
    switch (event.type) {
      case "added":
        host.send(
          mutationEvent({
            kind: "added",
            mutationId,
            mutation: serializeMutation(mutation, limits),
          }),
        );
        return;
      case "removed":
        host.send(mutationEvent({ kind: "removed", mutationId }));
        return;
      case "updated":
        host.send(
          mutationEvent({ kind: "updated", mutationId, delta: mutationDelta(mutation, limits) }),
        );
        return;
      default:
        return;
    }
  }

  function sendSnapshot(): void {
    const message: QueriesSnapshotMessage = {
      v: 1,
      type: "queries.snapshot",
      payload: {
        queries: queryClient
          .getQueryCache()
          .getAll()
          .map((query) => serializeQuery(query, limits)),
        mutations: queryClient
          .getMutationCache()
          .getAll()
          .map((mutation) => serializeMutation(mutation, limits)),
        online: onlineManager ? onlineManager.isOnline() : true,
      },
    };
    host.send(message);
  }

  function attach(): void {
    if (unsubscribers.length > 0) return;
    onlineManager ??= resolveOnlineManager(host.log);
    unsubscribers = [
      queryClient.getQueryCache().subscribe(onQueryEvent),
      queryClient.getMutationCache().subscribe(onMutationEvent),
    ];
    if (onlineManager) {
      unsubscribers.push(
        onlineManager.subscribe((online) => {
          if (!host.isReady()) return;
          host.send({ v: 1, type: "online.status", payload: { online } });
        }),
      );
    }
  }

  function detach(): void {
    for (const unsubscribe of unsubscribers) unsubscribe();
    unsubscribers = [];
  }

  function handle(message: LensMessage): boolean {
    switch (message.type) {
      case "snapshot.request":
        sendSnapshot();
        return true;
      case "query.action":
        void runQueryAction(queryClient, message.payload).then((payload) => {
          host.send({ v: 1, type: "action.result", id: message.id, payload });
        });
        return true;
      case "online.set":
        if (onlineManager) onlineManager.setOnline(message.payload.online);
        else host.log.warn("online.set ignored: onlineManager unavailable");
        return true;
      default:
        return false;
    }
  }

  return { attach, detach, sendSnapshot, handle };
}
