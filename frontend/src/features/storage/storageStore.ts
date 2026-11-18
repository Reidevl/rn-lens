import { create } from "zustand";

import type { Backend, StorageDevice, StorageStore, StorageValue } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

type Loaded =
  | { state: "loading" }
  | { state: "ready"; value: StorageValue }
  | { state: "error"; message: string };

interface StorageState {
  devices: StorageDevice[];
  deviceKey: string | null;
  stores: StorageStore[];
  storeId: string | null;
  selectedKey: string | null;
  /** The opened value, keyed by valueId(device, store, key). */
  value: { id: string; loaded: Loaded } | null;
  filter: string;
  busy: boolean;
  error: string | null;
  notice: string | null;
  selectDevice: (key: string) => void;
  selectStore: (id: string) => void;
  selectKey: (key: string | null) => void;
  setFilter: (filter: string) => void;
  setNotice: (notice: string | null) => void;
  setError: (error: string | null) => void;
  setBusy: (busy: boolean) => void;
  reset: () => void;
}

const initial = {
  devices: [] as StorageDevice[],
  deviceKey: null as string | null,
  stores: [] as StorageStore[],
  storeId: null as string | null,
  selectedKey: null as string | null,
  value: null as { id: string; loaded: Loaded } | null,
  filter: "",
  busy: false,
  error: null as string | null,
  notice: null as string | null,
};

export const useStorageStore = create<StorageState>((set) => ({
  ...initial,
  selectDevice: (deviceKey) => set({ deviceKey, storeId: null, selectedKey: null, value: null }),
  selectStore: (storeId) => set({ storeId, selectedKey: null, value: null, filter: "" }),
  selectKey: (selectedKey) => set({ selectedKey }),
  setFilter: (filter) => set({ filter }),
  setNotice: (notice) => set({ notice }),
  setError: (error) => set({ error }),
  setBusy: (busy) => set({ busy }),
  reset: () => set(initial),
}));

/**
 * Pulls devices and the chosen device's stores. Picks the first device and
 * store when none is chosen, and forgets a key that no longer exists.
 */
export async function pullStorage(backend: Backend): Promise<void> {
  const devices = await backend.storage.devices();
  let { deviceKey } = useStorageStore.getState();
  if (!deviceKey || !devices.some((d) => d.key === deviceKey)) deviceKey = devices[0]?.key ?? null;
  const stores = deviceKey ? await backend.storage.stores(deviceKey) : [];

  const latest = useStorageStore.getState();
  if (
    latest.deviceKey &&
    latest.deviceKey !== deviceKey &&
    devices.some((d) => d.key === latest.deviceKey)
  ) {
    return; // the user picked another device while we awaited; its pull follows
  }
  let { storeId, selectedKey } = latest;
  if (!storeId || !stores.some((s) => s.id === storeId)) storeId = stores[0]?.id ?? null;
  const store = stores.find((s) => s.id === storeId);
  if (selectedKey && !store?.keys?.some((k) => k.key === selectedKey)) selectedKey = null;
  useStorageStore.setState({
    devices,
    deviceKey,
    stores,
    storeId,
    selectedKey,
    value: selectedKey ? latest.value : null,
  });
}

/** Identifies an opened value; the device is part of it. */
export const valueId = (deviceKey: string, storeId: string, key: string) =>
  JSON.stringify([deviceKey, storeId, key]);

/**
 * Reads the selected key's value from the device. A quiet reload (after a
 * storage:changed notice) keeps showing the current value until the new one
 * arrives.
 */
export async function loadValue(backend: Backend, opts: { quiet?: boolean } = {}): Promise<void> {
  const { deviceKey, storeId, selectedKey, value } = useStorageStore.getState();
  if (!deviceKey || !storeId || !selectedKey) return;
  const id = valueId(deviceKey, storeId, selectedKey);
  if (!opts.quiet || value?.id !== id) {
    useStorageStore.setState({ value: { id, loaded: { state: "loading" } } });
  }
  let loaded: Loaded;
  try {
    loaded = { state: "ready", value: await backend.storage.get(deviceKey, storeId, selectedKey) };
  } catch (err) {
    loaded = { state: "error", message: errorMessage(err) };
  }
  if (useStorageStore.getState().value?.id === id)
    useStorageStore.setState({ value: { id, loaded } });
}

/** Runs a write and surfaces its error or notice in the panel. */
export async function runStorageAction(action: () => Promise<string | void>): Promise<boolean> {
  const store = useStorageStore.getState();
  store.setBusy(true);
  store.setError(null);
  store.setNotice(null);
  try {
    const notice = await action();
    if (notice) useStorageStore.getState().setNotice(notice);
    return true;
  } catch (err) {
    useStorageStore.getState().setError(errorMessage(err));
    return false;
  } finally {
    useStorageStore.getState().setBusy(false);
  }
}
