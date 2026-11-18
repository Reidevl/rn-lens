import { create } from "zustand";

import type { Backend, PerformanceStatus } from "@/backend";

export const idlePerformance: PerformanceStatus = {
  tracing: false,
  profiling: false,
  snapshotting: false,
  fastRefresh: { phase: "off" },
};

interface PerformanceStore {
  status: PerformanceStatus;
  setStatus(status: PerformanceStatus): void;
}

export const usePerformanceStore = create<PerformanceStore>((set) => ({
  status: idlePerformance,
  setStatus: (status) => set({ status }),
}));

/** Pulls the recordings in progress and the Fast Refresh status. */
export async function pullPerformance(backend: Backend): Promise<void> {
  usePerformanceStore.getState().setStatus(await backend.performance.status());
}
