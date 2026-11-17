import { create } from "zustand";

import type { Backend, ConsoleEntry, ConsoleLevel } from "@/backend";

/** Entries kept in memory; older ones are dropped like Go does. */
export const MAX_ENTRIES = 10_000;
const PAGE = 2_000;
const HISTORY_MAX = 100;

export type LevelFilter = Record<ConsoleLevel, boolean>;

export const defaultLevels: LevelFilter = {
  verbose: false,
  info: true,
  warning: true,
  error: true,
  system: true,
};

interface ConsoleState {
  entries: ConsoleEntry[];
  lastSeq: number;
  dropped: number;
  epoch: number;
  levels: LevelFilter;
  query: string;
  preserveLog: boolean;
  follow: boolean;
  history: string[];
  busy: boolean;
  error: string | null;
  setQuery: (query: string) => void;
  toggleLevel: (level: ConsoleLevel) => void;
  setPreserveLog: (on: boolean) => void;
  setFollow: (on: boolean) => void;
  pushHistory: (expression: string) => void;
  setBusy: (busy: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initial = {
  entries: [] as ConsoleEntry[],
  lastSeq: 0,
  dropped: 0,
  epoch: 1,
  levels: { ...defaultLevels },
  query: "",
  preserveLog: false,
  follow: true,
  history: [] as string[],
  busy: false,
  error: null as string | null,
};

export const useConsoleStore = create<ConsoleState>((set) => ({
  ...initial,
  setQuery: (query) => set({ query }),
  toggleLevel: (level) => set((s) => ({ levels: { ...s.levels, [level]: !s.levels[level] } })),
  setPreserveLog: (preserveLog) => set({ preserveLog }),
  setFollow: (follow) => set({ follow }),
  pushHistory: (expression) =>
    set((s) => {
      const next = [...s.history.filter((h) => h !== expression), expression];
      return { history: next.slice(-HISTORY_MAX) };
    }),
  setBusy: (busy) => set({ busy }),
  setError: (error) => set({ error }),
  reset: () => set({ ...initial, levels: { ...defaultLevels } }),
}));

/**
 * Pulls what changed since the last pull: Stats first, then only the missing
 * tail of entries (ADR-0003). Handles Go-side clears, drops and epoch resets.
 */
export async function pullConsole(backend: Backend): Promise<void> {
  const stats = await backend.console.stats();
  const state = useConsoleStore.getState();

  if (stats.len === 0) {
    useConsoleStore.setState({
      entries: [],
      lastSeq: stats.lastSeq,
      dropped: stats.dropped,
      epoch: stats.epoch,
    });
    return;
  }

  let from = state.lastSeq + 1;
  if (stats.oldestSeq > from) from = stats.oldestSeq;

  const fresh: ConsoleEntry[] = [];
  while (from <= stats.lastSeq) {
    const batch = await backend.console.range(from, PAGE);
    if (batch.length === 0) break;
    fresh.push(...batch);
    from = batch[batch.length - 1].seq + 1;
  }

  let entries = fresh.length ? [...state.entries, ...fresh] : state.entries;
  if (stats.epoch > state.epoch && !state.preserveLog) {
    // Keep the reset marker and everything after it.
    entries = entries.filter((e) => e.epoch >= stats.epoch);
  }
  if (entries.length > MAX_ENTRIES) entries = entries.slice(entries.length - MAX_ENTRIES);

  useConsoleStore.setState({
    entries,
    lastSeq: Math.max(state.lastSeq, stats.lastSeq),
    dropped: stats.dropped,
    epoch: stats.epoch,
  });
}

/** Runs a console action and surfaces its error in the store. */
export async function runConsoleAction(action: () => Promise<void>): Promise<void> {
  const store = useConsoleStore.getState();
  store.setBusy(true);
  store.setError(null);
  try {
    await action();
  } catch (err) {
    useConsoleStore.getState().setError(err instanceof Error ? err.message : String(err));
  } finally {
    useConsoleStore.getState().setBusy(false);
  }
}

/** Applies the level and text filters. */
export function visibleEntries(
  entries: ConsoleEntry[],
  levels: LevelFilter,
  query: string,
): ConsoleEntry[] {
  const q = query.trim().toLowerCase();
  return entries.filter((e) => levels[e.level] && (q === "" || e.text.toLowerCase().includes(q)));
}

/** Counts entries per level for the filter chips. */
export function countByLevel(entries: ConsoleEntry[]): Record<ConsoleLevel, number> {
  const counts: Record<ConsoleLevel, number> = {
    verbose: 0,
    info: 0,
    warning: 0,
    error: 0,
    system: 0,
  };
  for (const e of entries) counts[e.level]++;
  return counts;
}
