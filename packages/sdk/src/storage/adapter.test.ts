import type { StorageActionPayload, StorageResultPayload, StorageStore } from "@rnlens/protocol";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createLensClient, type LensClient } from "../client";
import { FakeLens, waitFor } from "../testing/fake-lens";
import { STORAGE_SNAPSHOT_MAX_KEYS, STORAGE_VALUE_MAX_BYTES } from "./adapter";
import { FakeAsyncStorage, FakeMMKV, FakeSecureStore } from "./fakes";
import type { LensStorageOptions } from "./types";

let lens: FakeLens;
let client: LensClient | undefined;
let nextId = 0;

beforeEach(async () => {
  lens = await FakeLens.start();
});

afterEach(async () => {
  client?.disconnect();
  client = undefined;
  await lens.close();
  expect(lens.invalid).toEqual([]);
});

async function connect(storage: LensStorageOptions): Promise<StorageStore[]> {
  client = createLensClient({
    url: lens.url,
    enabled: true,
    storage,
    backoff: { minMs: 20, maxMs: 40 },
  });
  client.connect();
  await lens.welcome();
  const snapshot = await lens.next("storage.snapshot");
  expect(snapshot.payload.storeId).toBeUndefined();
  return snapshot.payload.stores;
}

async function act(payload: Omit<StorageActionPayload, "id">): Promise<StorageResultPayload> {
  const id = `s${++nextId}`;
  lens.send({ v: 1, type: "storage.action", id, payload: { id, ...payload } });
  const result = await lens.next("storage.result");
  expect(result.payload.id).toBe(id);
  return result.payload;
}

const bytes = (...values: number[]) => new Uint8Array(values);

describe("storage snapshot", () => {
  it("lists the three stores with sorted keys on welcome", async () => {
    const stores = await connect({
      asyncStorage: new FakeAsyncStorage({ b: "two", a: "één" }),
      mmkv: { default: new FakeMMKV({ name: "Ada", count: 3, flag: true, blob: bytes(1, 2, 3) }) },
      secureStore: { module: new FakeSecureStore({ token: "t" }), keys: ["token", "refresh"] },
    });
    expect(stores).toEqual([
      {
        id: "async-storage",
        kind: "async-storage",
        name: "AsyncStorage",
        keys: [
          { key: "a", size: 5 },
          { key: "b", size: 3 },
        ],
        live: false,
      },
      {
        id: "mmkv:default",
        kind: "mmkv",
        name: "default",
        keys: [
          { key: "blob", size: 3 },
          { key: "count" },
          { key: "flag" },
          { key: "name", size: 3 },
        ],
        live: true,
      },
      {
        id: "secure-store",
        kind: "secure-store",
        name: "SecureStore",
        keys: [{ key: "refresh" }, { key: "token" }],
        live: false,
        declaredKeys: true,
      },
    ]);
  });

  it("omits AsyncStorage sizes above 500 keys instead of reading every value", async () => {
    const seed: Record<string, string> = {};
    for (let i = 0; i < 501; i++) seed[`k${i}`] = "v";
    const asyncStorage = new FakeAsyncStorage(seed);
    const [store] = await connect({ asyncStorage });
    expect(store.keys).toHaveLength(501);
    expect(store.keys[0]).toEqual({ key: "k0" });
    expect(asyncStorage.getItemCalls).toBe(0);
  });

  it(`keeps the first ${STORAGE_SNAPSHOT_MAX_KEYS} sorted keys and flags the store truncated`, async () => {
    const seed: Record<string, number> = {};
    for (let i = 0; i < STORAGE_SNAPSHOT_MAX_KEYS + 5; i++)
      seed[`k${String(i).padStart(5, "0")}`] = i;
    const [store] = await connect({ mmkv: { big: new FakeMMKV(seed) } });
    expect(store.keys).toHaveLength(STORAGE_SNAPSHOT_MAX_KEYS);
    expect(store.keys[0].key).toBe("k00000");
    expect(store.keys.at(-1)?.key).toBe("k09999");
    expect(store.truncated).toBe(true);
  });

  it("REFRESH answers ok and resends that store only", async () => {
    const asyncStorage = new FakeAsyncStorage({ a: "1" });
    await connect({ asyncStorage, mmkv: { m: new FakeMMKV() } });
    asyncStorage.data.set("b", "2");
    const result = await act({ type: "REFRESH", storeId: "async-storage" });
    expect(result).toEqual({ id: expect.any(String), ok: true });
    const snapshot = await lens.next("storage.snapshot");
    expect(snapshot.payload.storeId).toBe("async-storage");
    expect(snapshot.payload.stores.map((s) => s.id)).toEqual(["async-storage"]);
    expect(snapshot.payload.stores[0].keys.map((k) => k.key)).toEqual(["a", "b"]);
  });

  it("answers ok:false for an unknown store", async () => {
    await connect({ asyncStorage: new FakeAsyncStorage() });
    expect(await act({ type: "GET", storeId: "mmkv:nope", key: "a" })).toMatchObject({
      ok: false,
      error: 'unknown store "mmkv:nope"',
    });
  });
});

describe("exclude", () => {
  const exclude = ["secret", /^auth\./];

  it("hides matching keys from every store", async () => {
    const stores = await connect({
      asyncStorage: new FakeAsyncStorage({ secret: "x", "auth.token": "y", ok: "z" }),
      mmkv: { m: new FakeMMKV({ "auth.id": "1", visible: "2" }) },
      secureStore: { module: new FakeSecureStore(), keys: ["secret", "pin"] },
      exclude,
    });
    expect(stores.map((s) => s.keys.map((k) => k.key))).toEqual([["ok"], ["visible"], ["pin"]]);
  });

  it("refuses every action on an excluded key", async () => {
    const asyncStorage = new FakeAsyncStorage({ secret: "x" });
    await connect({ asyncStorage, exclude });
    for (const type of ["GET", "SET", "REMOVE"] as const) {
      const result = await act({
        type,
        storeId: "async-storage",
        key: "secret",
        value: { type: "string", value: "new" },
      });
      expect(result).toEqual({
        id: expect.any(String),
        ok: false,
        error: "key is excluded by the app",
      });
    }
    expect(asyncStorage.data.get("secret")).toBe("x");
  });

  it("CLEAR keeps excluded keys", async () => {
    const asyncStorage = new FakeAsyncStorage({ secret: "x", a: "1", b: "2" });
    const mmkv = new FakeMMKV({ "auth.id": "1", visible: "2" });
    await connect({ asyncStorage, mmkv: { m: mmkv }, exclude });
    expect((await act({ type: "CLEAR", storeId: "async-storage" })).ok).toBe(true);
    expect([...asyncStorage.data.keys()]).toEqual(["secret"]);
    expect((await act({ type: "CLEAR", storeId: "mmkv:m" })).ok).toBe(true);
    expect([...mmkv.data.keys()]).toEqual(["auth.id"]);
  });
});

describe("GET", () => {
  it("reads AsyncStorage and SecureStore strings", async () => {
    await connect({
      asyncStorage: new FakeAsyncStorage({ a: "hello" }),
      secureStore: { module: new FakeSecureStore({ token: "t0k" }), keys: ["token", "missing"] },
    });
    expect(await act({ type: "GET", storeId: "async-storage", key: "a" })).toMatchObject({
      ok: true,
      value: { type: "string", value: "hello", size: 5 },
    });
    expect(await act({ type: "GET", storeId: "secure-store", key: "token" })).toMatchObject({
      ok: true,
      value: { type: "string", value: "t0k", size: 3 },
    });
    expect(await act({ type: "GET", storeId: "secure-store", key: "missing" })).toMatchObject({
      ok: false,
      error: "not set",
    });
    expect(await act({ type: "GET", storeId: "async-storage", key: "nope" })).toMatchObject({
      ok: false,
      error: "not set",
    });
  });

  it("reads every MMKV value type", async () => {
    await connect({
      mmkv: { m: new FakeMMKV({ s: "x", n: 42.5, b: false, raw: bytes(0, 255, 16) }) },
    });
    const get = (key: string) => act({ type: "GET", storeId: "mmkv:m", key });
    expect((await get("s")).value).toEqual({ type: "string", value: "x", size: 1 });
    expect((await get("n")).value).toEqual({ type: "number", value: "42.5" });
    expect((await get("b")).value).toEqual({ type: "boolean", value: "false" });
    expect((await get("raw")).value).toEqual({ type: "bytes", value: "AP8Q", size: 3 });
    expect(await get("none")).toMatchObject({ ok: false, error: "not set" });
  });

  it(`answers a value above ${STORAGE_VALUE_MAX_BYTES} bytes as truncated`, async () => {
    const big = "é".repeat(STORAGE_VALUE_MAX_BYTES / 2 + 1);
    await connect({
      asyncStorage: new FakeAsyncStorage({ big, edge: "a".repeat(STORAGE_VALUE_MAX_BYTES) }),
    });
    expect(await act({ type: "GET", storeId: "async-storage", key: "big" })).toMatchObject({
      ok: true,
      value: { type: "string", value: "", truncated: true, size: STORAGE_VALUE_MAX_BYTES + 2 },
    });
    const edge = await act({ type: "GET", storeId: "async-storage", key: "edge" });
    expect(edge.value?.truncated).toBeUndefined();
    expect(edge.value?.value).toHaveLength(STORAGE_VALUE_MAX_BYTES);
  });
});

describe("writes on AsyncStorage and SecureStore", () => {
  it("SET, REMOVE and CLEAR write through and resend the store's snapshot", async () => {
    const asyncStorage = new FakeAsyncStorage({ a: "1" });
    await connect({ asyncStorage });

    expect(
      await act({
        type: "SET",
        storeId: "async-storage",
        key: "b",
        value: { type: "string", value: "2" },
      }),
    ).toEqual({ id: expect.any(String), ok: true });
    expect(await asyncStorage.getItem("b")).toBe("2");
    let snapshot = await lens.next("storage.snapshot");
    expect(snapshot.payload.storeId).toBe("async-storage");
    expect(snapshot.payload.stores[0].keys.map((k) => k.key)).toEqual(["a", "b"]);

    expect((await act({ type: "REMOVE", storeId: "async-storage", key: "a" })).ok).toBe(true);
    expect(asyncStorage.data.has("a")).toBe(false);
    snapshot = await lens.next("storage.snapshot");
    expect(snapshot.payload.stores[0].keys.map((k) => k.key)).toEqual(["b"]);

    expect((await act({ type: "CLEAR", storeId: "async-storage" })).ok).toBe(true);
    expect(asyncStorage.data.size).toBe(0);
    snapshot = await lens.next("storage.snapshot");
    expect(snapshot.payload.stores[0].keys).toEqual([]);
  });

  it("refuses non-string values on AsyncStorage and SecureStore", async () => {
    await connect({
      asyncStorage: new FakeAsyncStorage(),
      secureStore: { module: new FakeSecureStore(), keys: ["k"] },
    });
    expect(
      await act({
        type: "SET",
        storeId: "async-storage",
        key: "k",
        value: { type: "number", value: "1" },
      }),
    ).toMatchObject({ ok: false, error: "AsyncStorage stores strings" });
    expect(
      await act({
        type: "SET",
        storeId: "secure-store",
        key: "k",
        value: { type: "bytes", value: "AA==" },
      }),
    ).toMatchObject({ ok: false, error: "SecureStore stores strings" });
  });

  it("SecureStore writes declared keys only and CLEAR deletes every declared key", async () => {
    const secure = new FakeSecureStore({ a: "1", b: "2", other: "3" });
    await connect({ secureStore: { module: secure, keys: ["a", "b"] } });

    expect(
      await act({
        type: "SET",
        storeId: "secure-store",
        key: "a",
        value: { type: "string", value: "new" },
      }),
    ).toEqual({ id: expect.any(String), ok: true });
    expect(secure.data.get("a")).toBe("new");
    expect((await lens.next("storage.snapshot")).payload.storeId).toBe("secure-store");

    expect(
      await act({
        type: "SET",
        storeId: "secure-store",
        key: "x",
        value: { type: "string", value: "v" },
      }),
    ).toMatchObject({ ok: false, error: 'key "x" is not declared in storage.secureStore.keys' });

    expect((await act({ type: "REMOVE", storeId: "secure-store", key: "b" })).ok).toBe(true);
    expect(secure.data.has("b")).toBe(false);
    await lens.next("storage.snapshot");

    expect((await act({ type: "CLEAR", storeId: "secure-store" })).ok).toBe(true);
    expect([...secure.data.keys()]).toEqual(["other"]);
  });

  it("answers ok:false when SET has no key or value", async () => {
    await connect({ asyncStorage: new FakeAsyncStorage() });
    expect(
      await act({ type: "SET", storeId: "async-storage", value: { type: "string", value: "v" } }),
    ).toMatchObject({
      ok: false,
      error: "SET needs a key",
    });
    expect(await act({ type: "SET", storeId: "async-storage", key: "k" })).toMatchObject({
      ok: false,
      error: "SET needs a value",
    });
  });

  it("reports a storage API failure as ok:false", async () => {
    const asyncStorage = new FakeAsyncStorage();
    asyncStorage.setItem = () => Promise.reject(new Error("disk full"));
    await connect({ asyncStorage });
    expect(
      await act({
        type: "SET",
        storeId: "async-storage",
        key: "k",
        value: { type: "string", value: "v" },
      }),
    ).toMatchObject({ ok: false, error: "disk full" });
  });
});

describe("MMKV", () => {
  it("SET writes every value type and the listener streams storage.event", async () => {
    const mmkv = new FakeMMKV();
    await connect({ mmkv: { m: mmkv } });
    const set = (key: string, type: "string" | "number" | "boolean" | "bytes", value: string) =>
      act({ type: "SET", storeId: "mmkv:m", key, value: { type, value } });

    expect((await set("s", "string", "héllo")).ok).toBe(true);
    expect(mmkv.data.get("s")).toBe("héllo");
    expect((await lens.next("storage.event")).payload).toEqual({
      storeId: "mmkv:m",
      kind: "set",
      key: "s",
      size: 6,
    });

    expect((await set("n", "number", "42")).ok).toBe(true);
    expect(mmkv.data.get("n")).toBe(42);
    expect((await set("b", "boolean", "true")).ok).toBe(true);
    expect(mmkv.data.get("b")).toBe(true);
    expect((await set("raw", "bytes", "AP8Q")).ok).toBe(true);
    expect(mmkv.data.get("raw")).toEqual(bytes(0, 255, 16));
    await lens.sync();
    const events = lens.drain("storage.event").map((e) => e.type === "storage.event" && e.payload);
    expect(events).toEqual([
      { storeId: "mmkv:m", kind: "set", key: "n" },
      { storeId: "mmkv:m", kind: "set", key: "b" },
      { storeId: "mmkv:m", kind: "set", key: "raw", size: 3 },
    ]);
    // A live store sends no snapshot after a write.
    expect(lens.drain("storage.snapshot")).toEqual([]);

    expect(await set("bad", "number", "abc")).toMatchObject({
      ok: false,
      error: "value is not a number",
    });
    expect(await set("bad", "boolean", "yes")).toMatchObject({
      ok: false,
      error: "value is not a boolean",
    });
    expect(await set("bad", "bytes", "@@@")).toMatchObject({
      ok: false,
      error: "value is not base64",
    });
    expect(mmkv.data.has("bad")).toBe(false);
  });

  it("app-side changes reach Lens; REMOVE sends removed and CLEAR sends one cleared", async () => {
    const mmkv = new FakeMMKV({ a: "1", b: "2" });
    await connect({ mmkv: { m: mmkv } });

    mmkv.set("c", "33");
    expect((await lens.next("storage.event")).payload).toEqual({
      storeId: "mmkv:m",
      kind: "set",
      key: "c",
      size: 2,
    });

    expect((await act({ type: "REMOVE", storeId: "mmkv:m", key: "a" })).ok).toBe(true);
    expect((await lens.next("storage.event")).payload).toEqual({
      storeId: "mmkv:m",
      kind: "removed",
      key: "a",
    });

    expect((await act({ type: "CLEAR", storeId: "mmkv:m" })).ok).toBe(true);
    await lens.sync();
    const events = lens.drain("storage.event").map((e) => e.type === "storage.event" && e.payload);
    expect(events).toEqual([{ storeId: "mmkv:m", kind: "cleared" }]);
    expect(mmkv.data.size).toBe(0);
  });

  it("does not report changes to excluded keys", async () => {
    const mmkv = new FakeMMKV();
    await connect({ mmkv: { m: mmkv }, exclude: [/^secret/] });
    mmkv.set("secret.pin", "1234");
    mmkv.set("open", "x");
    expect((await lens.next("storage.event")).payload.key).toBe("open");
    await lens.sync();
    expect(lens.drain("storage.event")).toEqual([]);
  });

  it("detach removes the listener", async () => {
    const mmkv = new FakeMMKV();
    await connect({ mmkv: { m: mmkv } });
    expect(mmkv.listeners.size).toBe(1);
    client?.disconnect();
    await waitFor(() => client?.state === "idle");
    expect(mmkv.listeners.size).toBe(0);
  });
});

describe("MMKV v4", () => {
  // react-native-mmkv v4 renamed delete(key) to remove(key).
  class FakeMMKVv4 extends FakeMMKV {
    declare delete: never;
    remove(key: string): boolean {
      const had = this.data.has(key);
      FakeMMKV.prototype.delete.call(this, key);
      return had;
    }
  }

  it("REMOVE uses remove() when the instance has no delete()", async () => {
    const mmkv = new FakeMMKVv4({ gone: "x" });
    Object.defineProperty(mmkv, "delete", { value: undefined });
    await connect({ mmkv: { m: mmkv } });
    expect((await act({ type: "REMOVE", storeId: "mmkv:m", key: "gone" })).ok).toBe(true);
    expect(mmkv.data.has("gone")).toBe(false);
  });
});
