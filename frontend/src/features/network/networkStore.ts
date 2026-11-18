import { create } from "zustand";

import type { Backend, NetworkAvailability, NetworkSummary } from "@/backend";

import { categoryOf, DEFAULT_SORT, type Category, type Sort, type SortKey } from "./presentation";

const PAGE = 1_000;

interface NetworkState {
  /** Every row Go still holds, by ascending seq. */
  rows: NetworkSummary[];
  version: number;
  epoch: number;
  cleared: number;
  dropped: number;
  availability: NetworkAvailability;
  filter: string;
  /** Empty means every category. */
  categories: Category[];
  preserveLog: boolean;
  sort: Sort;
  selectedSeq: number | null;
  busy: boolean;
  error: string | null;
  notice: string | null;
  setFilter: (filter: string) => void;
  toggleCategory: (category: Category) => void;
  clearCategories: () => void;
  setPreserveLog: (preserve: boolean) => void;
  /** Sorts by key; the same key again flips the direction. */
  setSort: (key: SortKey) => void;
  select: (seq: number | null) => void;
  setBusy: (busy: boolean) => void;
  setError: (error: string | null) => void;
  setNotice: (notice: string | null) => void;
  reset: () => void;
}

const initial = {
  rows: [] as NetworkSummary[],
  version: 0,
  epoch: 1,
  cleared: 0,
  dropped: 0,
  availability: { checked: false, supported: false } as NetworkAvailability,
  filter: "",
  categories: [] as Category[],
  preserveLog: false,
  sort: DEFAULT_SORT,
  selectedSeq: null as number | null,
  busy: false,
  error: null as string | null,
  notice: null as string | null,
};

export const useNetworkStore = create<NetworkState>((set) => ({
  ...initial,
  setFilter: (filter) => set({ filter }),
  toggleCategory: (category) =>
    set((s) => ({
      categories: s.categories.includes(category)
        ? s.categories.filter((c) => c !== category)
        : [...s.categories, category],
    })),
  clearCategories: () => set({ categories: [] }),
  setPreserveLog: (preserveLog) => set({ preserveLog }),
  setSort: (key) =>
    set((s) => ({
      sort:
        s.sort.key === key
          ? { key, dir: s.sort.dir === "asc" ? "desc" : "asc" }
          : { key, dir: key === "start" ? "desc" : "asc" },
    })),
  select: (selectedSeq) => set({ selectedSeq }),
  setBusy: (busy) => set({ busy }),
  setError: (error) => set({ error }),
  setNotice: (notice) => set({ notice }),
  reset: () => set(initial),
}));

/**
 * Pulls what changed since the last pull: Stats first, then every row whose
 * version is newer, merged by seq. A new `cleared` value restarts from zero;
 * rows below Go's oldest seq were evicted and are dropped here too.
 */
export async function pullNetwork(backend: Backend): Promise<void> {
  const [stats, availability] = await Promise.all([
    backend.network.stats(),
    backend.network.availability(),
  ]);
  let state = useNetworkStore.getState();
  const restart = stats.cleared !== state.cleared;
  let version = restart ? 0 : state.version;
  const changed: NetworkSummary[] = [];
  for (;;) {
    const page = await backend.network.since(version, PAGE);
    changed.push(...page);
    if (page.length > 0) version = page[page.length - 1].version;
    if (page.length < PAGE) break;
  }

  // Re-read: a clear or another pull may have landed while we awaited.
  state = useNetworkStore.getState();
  const bySeq = new Map<number, NetworkSummary>();
  if (!restart && stats.cleared === state.cleared) {
    for (const r of state.rows) bySeq.set(r.seq, r);
  }
  for (const r of changed) bySeq.set(r.seq, r);
  // Stats were read before the rows: anything newer than its lastSeq arrived
  // in between and is kept; only older rows are checked against eviction.
  const rows = [...bySeq.values()]
    .filter((r) => r.seq > stats.lastSeq || (stats.len > 0 && r.seq >= stats.oldestSeq))
    .sort((a, b) => a.seq - b.seq);

  useNetworkStore.setState({
    rows,
    version: Math.max(version, restart ? 0 : state.version),
    epoch: stats.epoch,
    cleared: stats.cleared,
    dropped: stats.dropped,
    availability,
    selectedSeq: rows.some((r) => r.seq === state.selectedSeq) ? state.selectedSeq : null,
  });
}

/** Rows the table shows after the epoch, category and text filters. */
export function visibleRows(
  rows: NetworkSummary[],
  opts: { epoch: number; preserveLog: boolean; categories: Category[]; filter: string },
): NetworkSummary[] {
  const needle = opts.filter.trim().toLowerCase();
  return rows.filter(
    (r) =>
      (opts.preserveLog || r.epoch === opts.epoch) &&
      (opts.categories.length === 0 || opts.categories.includes(categoryOf(r))) &&
      (needle === "" || r.url.toLowerCase().includes(needle)),
  );
}
