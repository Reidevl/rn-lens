import { beforeEach, describe, expect, it } from "vitest";

import { createMockBackend, type StorageDevice } from "@/backend";

import { loadValue, pullStorage, useStorageStore } from "./storageStore";

const deviceA: StorageDevice = {
  key: "a|1",
  sessionId: "s1",
  app: { id: "a", platform: "ios" },
  device: { id: "1" },
  sdkVersion: "0.1.0",
  connectedAt: "2026-01-01T00:00:00Z",
};
const deviceB: StorageDevice = {
  ...deviceA,
  key: "b|1",
  sessionId: "s2",
  app: { id: "b", platform: "ios" },
};

describe("loadValue", () => {
  beforeEach(() => useStorageStore.getState().reset());

  it("never shows a value read from another device for the same store and key", async () => {
    const backend = createMockBackend();
    const store = {
      id: "async-storage",
      kind: "async-storage" as const,
      name: "AsyncStorage",
      keys: [{ key: "theme" }],
    };
    backend.connectStorage(deviceA, [store], {
      "async-storage/theme": { type: "string", value: "from A" },
    });
    backend.connectStorage(deviceB, [store], {
      "async-storage/theme": { type: "string", value: "from B" },
    });
    await pullStorage(backend);
    useStorageStore.getState().selectKey("theme");

    let releaseA: () => void = () => {};
    const get = backend.storage.get.bind(backend.storage);
    backend.storage.get = async (deviceKey, storeId, key) => {
      if (deviceKey === deviceA.key) await new Promise<void>((resolve) => (releaseA = resolve));
      return get(deviceKey, storeId, key);
    };
    const slow = loadValue(backend); // device A, still pending

    useStorageStore.getState().selectDevice(deviceB.key);
    await pullStorage(backend);
    useStorageStore.getState().selectKey("theme");
    await loadValue(backend);
    releaseA();
    await slow;

    expect(useStorageStore.getState().value?.loaded).toEqual({
      state: "ready",
      value: { type: "string", value: "from B" },
    });
  });
});
