import type { LensMessage, QueryActionType, StorageActionType } from "./types";

/** Protocol major this package describes. */
export const PROTOCOL_VERSION = 1 as const;

/** Every `type` value of the envelope, in schema order. */
export const MESSAGE_TYPES = [
  "hello",
  "queries.snapshot",
  "query.event",
  "mutation.event",
  "online.status",
  "action.result",
  "pong",
  "welcome",
  "snapshot.request",
  "query.action",
  "online.set",
  "ping",
  "storage.snapshot",
  "storage.event",
  "storage.action",
  "storage.result",
  "plugin.register",
  "plugin.view",
  "plugin.action",
  "plugin.result",
] as const;

export type MessageType = (typeof MESSAGE_TYPES)[number];

/** Every devtools action Lens can ask the device to run. */
export const QUERY_ACTION_TYPES: readonly QueryActionType[] = [
  "REFETCH",
  "INVALIDATE",
  "RESET",
  "REMOVE",
  "SET_QUERY_DATA",
  "TRIGGER_ERROR",
  "RESTORE_ERROR",
  "TRIGGER_LOADING",
  "RESTORE_LOADING",
  "CLEAR_QUERY_CACHE",
  "CLEAR_MUTATION_CACHE",
];

/** Every storage action Lens can ask the device to run. */
export const STORAGE_ACTION_TYPES: readonly StorageActionType[] = [
  "GET",
  "SET",
  "REMOVE",
  "CLEAR",
  "REFRESH",
];

/** Every block kind a plugin panel can contain, in schema order. */
export const PLUGIN_BLOCK_TYPES = ["text", "keyValue", "table", "json", "log", "actions"] as const;

export type PluginBlockType = (typeof PLUGIN_BLOCK_TYPES)[number];

/** Cheap structural check for the envelope; full validation is the schema's job. */
export function isLensMessage(value: unknown): value is LensMessage {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    v.v === PROTOCOL_VERSION &&
    typeof v.type === "string" &&
    (MESSAGE_TYPES as readonly string[]).includes(v.type) &&
    typeof v.payload === "object" &&
    v.payload !== null
  );
}
