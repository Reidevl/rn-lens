import { create } from "zustand";

import type { Backend, PluginDevice, PluginInfo } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

interface PluginsState {
  devices: PluginDevice[];
  deviceKey: string | null;
  plugins: PluginInfo[];
  pluginId: string | null;
  /** The action in flight, as "<blockId>/<actionId>/<rowId>". */
  running: string | null;
  error: string | null;
  notice: string | null;
  selectDevice: (key: string) => void;
  selectPlugin: (id: string) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initial = {
  devices: [] as PluginDevice[],
  deviceKey: null as string | null,
  plugins: [] as PluginInfo[],
  pluginId: null as string | null,
  running: null as string | null,
  error: null as string | null,
  notice: null as string | null,
};

export const usePluginsStore = create<PluginsState>((set) => ({
  ...initial,
  selectDevice: (deviceKey) => set({ deviceKey, pluginId: null, notice: null }),
  selectPlugin: (pluginId) => set({ pluginId, notice: null }),
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/**
 * Pulls devices and the chosen device's plugins. Picks the first device and
 * plugin when none is chosen or the chosen one is gone.
 */
export async function pullPlugins(backend: Backend): Promise<void> {
  const devices = await backend.plugins.devices();
  let { deviceKey } = usePluginsStore.getState();
  if (!deviceKey || !devices.some((d) => d.key === deviceKey)) deviceKey = devices[0]?.key ?? null;
  const plugins = deviceKey ? await backend.plugins.plugins(deviceKey) : [];

  const latest = usePluginsStore.getState();
  if (
    latest.deviceKey &&
    latest.deviceKey !== deviceKey &&
    devices.some((d) => d.key === latest.deviceKey)
  ) {
    return; // the user picked another device while we awaited; its pull follows
  }
  let { pluginId } = latest;
  if (!pluginId || !plugins.some((p) => p.id === pluginId)) pluginId = plugins[0]?.id ?? null;
  usePluginsStore.setState({ devices, deviceKey, plugins, pluginId });
}

/** Sends a click to the app and shows its message or error. */
export async function runPluginAction(
  backend: Backend,
  action: { pluginId: string; blockId: string; actionId: string; rowId?: string },
): Promise<void> {
  const { deviceKey, running } = usePluginsStore.getState();
  if (!deviceKey || running) return;
  const id = [action.blockId, action.actionId, action.rowId ?? ""].join("/");
  usePluginsStore.setState({ running: id, error: null, notice: null });
  try {
    const message = await backend.plugins.action(
      deviceKey,
      action.pluginId,
      action.blockId,
      action.actionId,
      action.rowId,
    );
    // Only a non-empty string is a message: anything else from the bridge is not.
    usePluginsStore.setState({ notice: typeof message === "string" && message ? message : null });
  } catch (err) {
    usePluginsStore.setState({ error: errorMessage(err) });
  } finally {
    usePluginsStore.setState({ running: null });
  }
}
