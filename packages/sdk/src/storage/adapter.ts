import type {
  LensMessage,
  StorageActionPayload,
  StorageEventPayload,
  StorageKey,
  StorageResultPayload,
  StorageStore,
  StorageValue,
} from "@rnlens/protocol";

import { jsonByteLength } from "../serialize";
import type { AdapterHost } from "../tanstack/adapter";
import { decodeBase64, encodeBase64 } from "./base64";
import type { AsyncStorageLike, LensStorageOptions, MMKVLike, SecureStoreLike } from "./types";

/** A GET value whose text form is larger is answered empty with `truncated: true`. */
export const STORAGE_VALUE_MAX_BYTES = 1024 * 1024;
/** A store with more keys lists the first ones (sorted) with `truncated: true`. */
export const STORAGE_SNAPSHOT_MAX_KEYS = 10_000;
/** AsyncStorage sizes need a read per key; above this many keys they are omitted. */
export const ASYNC_STORAGE_SIZED_KEYS_MAX = 500;

export interface StorageAdapter {
  attach(): void;
  detach(): void;
  sendSnapshot(): void;
  /** Handles an inbound frame; returns true when the adapter consumed it. */
  handle(message: LensMessage): boolean;
}

type StoreInfo = Omit<StorageStore, "keys" | "truncated">;

/** One store behind a uniform surface; keys reaching it are already allowed. */
interface StoreDriver {
  info: StoreInfo;
  /** Every key of the store, excluded ones included. */
  keys(): Promise<string[]>;
  /** Builds the snapshot entries of `keys`; `total` is the store's visible key count. */
  describe(keys: string[], total: number): Promise<StorageKey[]>;
  get(key: string): Promise<StorageValue | undefined>;
  /** Returns an error message when the value cannot be stored. */
  set(key: string, value: StorageValue): Promise<string | undefined>;
  remove(key: string): Promise<void>;
  clear(visibleKeys: string[], hasHidden: boolean): Promise<void>;
  /** Returns an error message when the store refuses the key. */
  checkKey?(key: string): string | undefined;
  /** Live stores report their own changes; returns the unsubscribe function. */
  subscribe?(): () => void;
}

const utf8 = jsonByteLength;

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/** Caps a value's text form at STORAGE_VALUE_MAX_BYTES. */
function capValue(
  type: StorageValue["type"],
  text: () => string,
  textBytes: number,
  size?: number,
): StorageValue {
  if (textBytes > STORAGE_VALUE_MAX_BYTES) {
    return { type, value: "", truncated: true, size: size ?? textBytes };
  }
  const value: StorageValue = { type, value: text() };
  if (size !== undefined) value.size = size;
  return value;
}

function stringValue(text: string): StorageValue {
  const bytes = utf8(text);
  return capValue("string", () => text, bytes, bytes);
}

function bytesValue(raw: ArrayBuffer | Uint8Array): StorageValue {
  const view = raw instanceof Uint8Array ? raw : new Uint8Array(raw);
  const base64Length = 4 * Math.ceil(view.byteLength / 3);
  return capValue("bytes", () => encodeBase64(view), base64Length, view.byteLength);
}

function stringOnly(name: string, value: StorageValue): string | undefined {
  return value.type === "string" ? undefined : `${name} stores strings`;
}

function asyncStorageDriver(storage: AsyncStorageLike): StoreDriver {
  return {
    info: { id: "async-storage", kind: "async-storage", name: "AsyncStorage", live: false },
    keys: async () => [...(await storage.getAllKeys())],
    async describe(keys, total) {
      if (total > ASYNC_STORAGE_SIZED_KEYS_MAX) return keys.map((key) => ({ key }));
      return Promise.all(
        keys.map(async (key) => {
          const value = await storage.getItem(key);
          return value === null ? { key } : { key, size: utf8(value) };
        }),
      );
    },
    async get(key) {
      const value = await storage.getItem(key);
      return value === null ? undefined : stringValue(value);
    },
    async set(key, value) {
      const refused = stringOnly("AsyncStorage", value);
      if (refused) return refused;
      await storage.setItem(key, value.value);
      return undefined;
    },
    remove: (key) => storage.removeItem(key),
    async clear(visibleKeys, hasHidden) {
      if (!hasHidden && storage.clear) return storage.clear();
      if (storage.multiRemove) return storage.multiRemove(visibleKeys);
      for (const key of visibleKeys) await storage.removeItem(key);
    },
  };
}

/**
 * MMKV does not expose an entry's type. The typed getters are tried in order
 * (string, number, boolean, bytes) and the first defined answer wins:
 * react-native-mmkv v3 returns undefined from `getString` for non-string
 * entries, but a number or boolean entry can still be read by the other
 * numeric getter, so number wins over boolean.
 */
function readMMKV(mmkv: MMKVLike, key: string): StorageValue | undefined {
  const text = mmkv.getString(key);
  if (text !== undefined) return stringValue(text);
  if (!mmkv.contains(key)) return undefined;
  const number = mmkv.getNumber(key);
  if (number !== undefined) return { type: "number", value: JSON.stringify(number) };
  const boolean = mmkv.getBoolean(key);
  if (boolean !== undefined) return { type: "boolean", value: String(boolean) };
  const buffer = mmkv.getBuffer(key);
  if (buffer !== undefined) return bytesValue(buffer);
  return undefined;
}

/** Size of a string (UTF-8) or bytes entry; numbers and booleans have none. */
function mmkvSize(mmkv: MMKVLike, key: string): number | undefined {
  const text = mmkv.getString(key);
  if (text !== undefined) return utf8(text);
  if (!mmkv.contains(key)) return undefined;
  if (mmkv.getNumber(key) !== undefined || mmkv.getBoolean(key) !== undefined) return undefined;
  return mmkv.getBuffer(key)?.byteLength;
}

function parseMMKVValue(
  value: StorageValue,
): string | number | boolean | Uint8Array | { error: string } {
  switch (value.type) {
    case "string":
      return value.value;
    case "number": {
      const parsed = value.value.trim() === "" ? NaN : Number(value.value);
      return Number.isFinite(parsed) ? parsed : { error: "value is not a number" };
    }
    case "boolean":
      if (value.value === "true") return true;
      if (value.value === "false") return false;
      return { error: "value is not a boolean" };
    case "bytes":
      return decodeBase64(value.value) ?? { error: "value is not base64" };
    default:
      return { error: `unsupported value type "${String((value as StorageValue).type)}"` };
  }
}

function mmkvDriver(
  name: string,
  mmkv: MMKVLike,
  emit: (payload: StorageEventPayload) => void,
  isVisible: (key: string) => boolean,
): StoreDriver {
  const id = `mmkv:${name}`;
  let clearing = false;
  return {
    info: { id, kind: "mmkv", name, live: true },
    keys: async () => mmkv.getAllKeys(),
    async describe(keys) {
      return keys.map((key) => {
        const size = mmkvSize(mmkv, key);
        return size === undefined ? { key } : { key, size };
      });
    },
    get: async (key) => readMMKV(mmkv, key),
    async set(key, value) {
      const parsed = parseMMKVValue(value);
      if (typeof parsed === "object" && "error" in parsed) return parsed.error;
      mmkv.set(key, parsed);
      return undefined;
    },
    remove: async (key) => removeMMKV(mmkv, key),
    async clear(visibleKeys, hasHidden) {
      // The listener would report every key; Lens gets one `cleared` instead.
      clearing = true;
      try {
        if (hasHidden) for (const key of visibleKeys) removeMMKV(mmkv, key);
        else mmkv.clearAll();
      } finally {
        clearing = false;
      }
      emit({ storeId: id, kind: "cleared" });
    },
    subscribe() {
      const subscription = mmkv.addOnValueChangedListener((key) => {
        if (clearing || !isVisible(key)) return;
        if (!mmkv.contains(key)) {
          emit({ storeId: id, kind: "removed", key });
          return;
        }
        const size = mmkvSize(mmkv, key);
        emit(
          size === undefined
            ? { storeId: id, kind: "set", key }
            : { storeId: id, kind: "set", key, size },
        );
      });
      return () => subscription.remove();
    },
  };
}

function secureStoreDriver(store: SecureStoreLike, declared: readonly string[]): StoreDriver {
  const declaredSet = new Set(declared);
  return {
    info: {
      id: "secure-store",
      kind: "secure-store",
      name: "SecureStore",
      live: false,
      declaredKeys: true,
    },
    keys: async () => [...declaredSet],
    // Reading a SecureStore value can prompt for authentication: no sizes.
    describe: async (keys) => keys.map((key) => ({ key })),
    async get(key) {
      const value = await store.getItemAsync(key);
      return value === null ? undefined : stringValue(value);
    },
    async set(key, value) {
      const refused = stringOnly("SecureStore", value);
      if (refused) return refused;
      await store.setItemAsync(key, value.value);
      return undefined;
    },
    remove: (key) => store.deleteItemAsync(key),
    async clear(visibleKeys) {
      for (const key of visibleKeys) await store.deleteItemAsync(key);
    },
    checkKey: (key) =>
      declaredSet.has(key) ? undefined : `key "${key}" is not declared in storage.secureStore.keys`,
  };
}

/**
 * Mirrors AsyncStorage, MMKV and SecureStore to Lens and runs `storage.action`
 * frames against them. Snapshots carry keys only; values travel on GET.
 */
export function createStorageAdapter(
  options: LensStorageOptions,
  host: AdapterHost,
): StorageAdapter {
  const exclude = options.exclude ?? [];
  const isVisible = (key: string): boolean =>
    !exclude.some((pattern) => {
      if (typeof pattern === "string") return pattern === key;
      pattern.lastIndex = 0;
      return pattern.test(key);
    });
  const emit = (payload: StorageEventPayload): void => {
    if (host.isReady()) host.send({ v: 1, type: "storage.event", payload });
  };

  const drivers: StoreDriver[] = [];
  if (options.asyncStorage) drivers.push(asyncStorageDriver(options.asyncStorage));
  for (const [name, mmkv] of Object.entries(options.mmkv ?? {})) {
    drivers.push(mmkvDriver(name, mmkv, emit, isVisible));
  }
  if (options.secureStore) {
    drivers.push(secureStoreDriver(options.secureStore.module, options.secureStore.keys));
  }
  const byId = new Map(drivers.map((driver) => [driver.info.id, driver]));
  let unsubscribers: Array<() => void> = [];

  async function visibleKeys(driver: StoreDriver): Promise<{ keys: string[]; hasHidden: boolean }> {
    const all = await driver.keys();
    const keys = all.filter(isVisible);
    return { keys, hasHidden: keys.length < all.length };
  }

  async function describeStore(driver: StoreDriver): Promise<StorageStore> {
    try {
      const { keys } = await visibleKeys(driver);
      keys.sort();
      const kept = keys.slice(0, STORAGE_SNAPSHOT_MAX_KEYS);
      const store: StorageStore = {
        ...driver.info,
        keys: await driver.describe(kept, keys.length),
      };
      if (keys.length > kept.length) {
        store.truncated = true;
        host.log.warn(
          `storage ${driver.info.id}: ${keys.length} keys, snapshot lists ${kept.length}`,
        );
      }
      return store;
    } catch (err) {
      host.log.warn(`storage ${driver.info.id}: cannot list keys`, err);
      return { ...driver.info, keys: [] };
    }
  }

  async function snapshot(only?: StoreDriver): Promise<void> {
    const stores = await Promise.all((only ? [only] : drivers).map(describeStore));
    if (!host.isReady()) return;
    const payload = only ? { stores, storeId: only.info.id } : { stores };
    host.send({ v: 1, type: "storage.snapshot", payload });
  }

  async function run(action: StorageActionPayload): Promise<Omit<StorageResultPayload, "id">> {
    const driver = byId.get(action.storeId);
    if (!driver) return { ok: false, error: `unknown store "${action.storeId}"` };
    if (action.type === "REFRESH") {
      await snapshot(driver);
      return { ok: true };
    }
    if (action.type === "CLEAR") {
      const { keys, hasHidden } = await visibleKeys(driver);
      await driver.clear(keys, hasHidden);
      if (!driver.info.live) await snapshot(driver);
      return { ok: true };
    }
    if (action.type !== "GET" && action.type !== "SET" && action.type !== "REMOVE") {
      return { ok: false, error: `unsupported action "${String(action.type)}"` };
    }
    const key = action.key;
    if (key === undefined) return { ok: false, error: `${action.type} needs a key` };
    if (!isVisible(key)) return { ok: false, error: "key is excluded by the app" };
    const refused = driver.checkKey?.(key);
    if (refused) return { ok: false, error: refused };

    if (action.type === "GET") {
      const value = await driver.get(key);
      return value === undefined ? { ok: false, error: "not set" } : { ok: true, value };
    }
    if (action.type === "SET") {
      if (!action.value) return { ok: false, error: "SET needs a value" };
      const error = await driver.set(key, action.value);
      if (error) return { ok: false, error };
    } else {
      await driver.remove(key);
    }
    // AsyncStorage and SecureStore have no listener: resend the store.
    if (!driver.info.live) await snapshot(driver);
    return { ok: true };
  }

  function handleAction(action: StorageActionPayload): void {
    void run(action)
      .catch((err: unknown) => ({ ok: false, error: errorMessage(err) }))
      .then((result) => {
        host.send({ v: 1, type: "storage.result", payload: { id: action.id, ...result } });
      });
  }

  function sendSnapshot(): void {
    void snapshot().catch((err: unknown) => host.log.warn("storage snapshot failed", err));
  }

  function attach(): void {
    if (unsubscribers.length > 0) return;
    for (const driver of drivers) {
      if (driver.subscribe) unsubscribers.push(driver.subscribe());
    }
  }

  function detach(): void {
    for (const unsubscribe of unsubscribers) unsubscribe();
    unsubscribers = [];
  }

  function handle(message: LensMessage): boolean {
    if (message.type !== "storage.action") return false;
    handleAction(message.payload);
    return true;
  }

  return { attach, detach, sendSnapshot, handle };
}

/** Deletes one key with whichever name the installed MMKV uses (v4 remove, v3 delete). */
function removeMMKV(mmkv: MMKVLike, key: string): void {
  if (typeof mmkv.remove === "function") mmkv.remove(key);
  else if (typeof mmkv.delete === "function") mmkv.delete(key);
  else throw new Error("this MMKV instance has neither remove() nor delete()");
}
