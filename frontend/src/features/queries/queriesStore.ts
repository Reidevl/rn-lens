import { create } from "zustand";

import type {
  Backend,
  MutationItem,
  QueryCounts,
  QueryItem,
  QueryLabel,
  SdkDevice,
} from "@/backend";

export type LabelFilter = Record<QueryLabel, boolean>;

const allLabels: LabelFilter = {
  fetching: true,
  fresh: true,
  stale: true,
  inactive: true,
  paused: true,
};
const emptyCounts: QueryCounts = {
  fetching: 0,
  fresh: 0,
  stale: 0,
  inactive: 0,
  paused: 0,
  total: 0,
};

interface QueriesState {
  devices: SdkDevice[];
  deviceKey: string | null;
  queries: QueryItem[];
  counts: QueryCounts;
  mutations: MutationItem[];
  selectedHash: string | null;
  selectedMutation: number | null;
  filter: string;
  labels: LabelFilter;
  busy: boolean;
  error: string | null;
  selectDevice: (key: string | null) => void;
  selectQuery: (hash: string | null) => void;
  selectMutation: (id: number | null) => void;
  setFilter: (filter: string) => void;
  toggleLabel: (label: QueryLabel) => void;
  setBusy: (busy: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initial = {
  devices: [] as SdkDevice[],
  deviceKey: null as string | null,
  queries: [] as QueryItem[],
  counts: emptyCounts,
  mutations: [] as MutationItem[],
  selectedHash: null as string | null,
  selectedMutation: null as number | null,
  filter: "",
  labels: { ...allLabels },
  busy: false,
  error: null as string | null,
};

export const useQueriesStore = create<QueriesState>((set) => ({
  ...initial,
  selectDevice: (deviceKey) => set({ deviceKey, selectedHash: null, selectedMutation: null }),
  selectQuery: (selectedHash) => set({ selectedHash }),
  selectMutation: (selectedMutation) => set({ selectedMutation }),
  setFilter: (filter) => set({ filter }),
  toggleLabel: (label) => set((s) => ({ labels: { ...s.labels, [label]: !s.labels[label] } })),
  setBusy: (busy) => set({ busy }),
  setError: (error) => set({ error }),
  reset: () => set({ ...initial, labels: { ...allLabels } }),
}));

/** Pulls devices, then the selected device's queries and mutations. */
export async function pullQueries(backend: Backend): Promise<void> {
  const devices = await backend.queries.devices();
  const state = useQueriesStore.getState();
  let deviceKey = state.deviceKey;
  if (!deviceKey || !devices.some((d) => d.key === deviceKey)) {
    deviceKey = devices[0]?.key ?? null;
  }
  if (!deviceKey) {
    useQueriesStore.setState({
      devices,
      deviceKey: null,
      queries: [],
      counts: emptyCounts,
      mutations: [],
    });
    return;
  }
  const [page, mutations] = await Promise.all([
    backend.queries.queries(deviceKey),
    backend.queries.mutations(deviceKey),
  ]);
  const selectedHash =
    state.selectedHash && page.queries.some((q) => q.queryHash === state.selectedHash)
      ? state.selectedHash
      : null;
  useQueriesStore.setState({
    devices,
    deviceKey,
    queries: page.queries,
    counts: page.counts,
    mutations,
    selectedHash,
  });
}

export async function runQueriesAction(
  backend: Backend,
  action: () => Promise<void>,
): Promise<void> {
  const store = useQueriesStore.getState();
  store.setBusy(true);
  store.setError(null);
  try {
    await action();
  } catch (err) {
    useQueriesStore.getState().setError(err instanceof Error ? err.message : String(err));
  } finally {
    useQueriesStore.getState().setBusy(false);
    await pullQueries(backend).catch(() => {});
  }
}

export function visibleQueries(
  queries: QueryItem[],
  labels: LabelFilter,
  filter: string,
): QueryItem[] {
  const f = filter.trim().toLowerCase();
  return queries.filter(
    (q) => labels[q.label] && (f === "" || q.queryHash.toLowerCase().includes(f)),
  );
}
