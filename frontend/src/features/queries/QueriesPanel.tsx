import { Search } from "lucide-react";
import { useMemo } from "react";

import { useBackend, type QueryLabel } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { useNow } from "@/shared/lib/useNow";
import { Button } from "@/shared/ui/atoms/Button";

import { DevicePicker } from "./DevicePicker";
import { NoSdk } from "./NoSdk";
import { ago, keyText, labelDot, labelText } from "./presentation";
import { QueryDetail } from "./QueryDetail";
import { runQueriesAction, useQueriesStore, visibleQueries } from "./queriesStore";
import { useQueriesSync } from "./useQueriesSync";

const LABELS: QueryLabel[] = ["fresh", "fetching", "paused", "stale", "inactive"];

export function QueriesPanel() {
  const backend = useBackend();
  useQueriesSync();
  const devices = useQueriesStore((s) => s.devices);
  const deviceKey = useQueriesStore((s) => s.deviceKey);
  const queries = useQueriesStore((s) => s.queries);
  const counts = useQueriesStore((s) => s.counts);
  const selectedHash = useQueriesStore((s) => s.selectedHash);
  const selectQuery = useQueriesStore((s) => s.selectQuery);
  const filter = useQueriesStore((s) => s.filter);
  const setFilter = useQueriesStore((s) => s.setFilter);
  const labels = useQueriesStore((s) => s.labels);
  const toggleLabel = useQueriesStore((s) => s.toggleLabel);
  const error = useQueriesStore((s) => s.error);
  const busy = useQueriesStore((s) => s.busy);
  const now = useNow();

  const visible = useMemo(() => visibleQueries(queries, labels, filter), [queries, labels, filter]);
  const selected = queries.find((q) => q.queryHash === selectedHash) ?? null;

  return (
    <section aria-labelledby="panel-queries-title" className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-3 border-b border-border px-3">
        <h1 id="panel-queries-title" className="text-sm font-semibold">
          Queries
        </h1>
        <DevicePicker devices={devices} deviceKey={deviceKey} />
        <div className="flex-1" />
        {deviceKey ? (
          <Button
            size="sm"
            disabled={busy}
            onClick={() =>
              void runQueriesAction(backend, () =>
                backend.queries.action(deviceKey, "CLEAR_QUERY_CACHE", "", ""),
              )
            }
          >
            Clear cache
          </Button>
        ) : null}
      </header>

      {error ? (
        <p
          role="alert"
          className="border-b border-destructive/60 bg-destructive/10 px-3 py-1 text-xs"
        >
          {error}
        </p>
      ) : null}

      {!deviceKey ? (
        <NoSdk />
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(18rem,2fr)_3fr]">
          <div className="flex min-h-0 flex-col border-r border-border">
            <div className="flex items-center gap-2 border-b border-border px-2 py-1.5">
              <label className="relative flex min-w-0 flex-1 items-center">
                <Search
                  aria-hidden="true"
                  className="pointer-events-none absolute left-2 size-3.5 text-muted-foreground"
                />
                <input
                  type="search"
                  aria-label="Filter queries by key"
                  placeholder="Filter by key"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  className="h-7 w-full rounded-md border border-border bg-background pr-2 pl-7 text-sm outline-none focus-visible:outline-2 focus-visible:outline-ring"
                />
              </label>
            </div>
            <div
              role="group"
              aria-label="Status filters"
              className="flex flex-wrap gap-1 border-b border-border px-2 py-1.5"
            >
              {LABELS.map((label) => (
                <button
                  key={label}
                  type="button"
                  aria-pressed={labels[label]}
                  aria-label={`${labelText[label]} (${counts[label]})`}
                  onClick={() => toggleLabel(label)}
                  className={cn(
                    "flex h-6 cursor-pointer items-center gap-1 rounded-md px-1.5 text-xs transition-colors duration-150",
                    labels[label]
                      ? "bg-muted text-foreground"
                      : "text-muted-foreground hover:bg-muted/60",
                  )}
                >
                  <span aria-hidden="true" className={cn("size-2 rounded-full", labelDot[label])} />
                  {labelText[label]}
                  <span className="tabular-nums opacity-70">{counts[label]}</span>
                </button>
              ))}
            </div>
            <ul role="listbox" aria-label="Queries" className="min-h-0 flex-1 overflow-auto">
              {visible.length === 0 ? (
                <li className="p-4 text-sm text-muted-foreground">
                  {queries.length === 0
                    ? "The app has no queries yet."
                    : "No queries match the filters."}
                </li>
              ) : (
                visible.map((q) => (
                  <li key={q.queryHash}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={q.queryHash === selectedHash}
                      onClick={() => selectQuery(q.queryHash)}
                      className={cn(
                        "flex w-full cursor-pointer items-center gap-2 border-b border-border/40 px-3 py-1.5 text-left text-sm",
                        q.queryHash === selectedHash ? "bg-muted" : "hover:bg-muted/50",
                      )}
                    >
                      <span
                        aria-hidden="true"
                        className={cn("size-2 shrink-0 rounded-full", labelDot[q.label])}
                      />
                      <span className="min-w-0 flex-1 truncate font-mono text-xs">
                        {keyText(q.queryKey)}
                      </span>
                      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                        {q.observersCount}
                      </span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {ago(q.state.dataUpdatedAt, now)}
                      </span>
                    </button>
                  </li>
                ))
              )}
            </ul>
          </div>
          <div className="min-h-0 overflow-hidden">
            {selected ? (
              <QueryDetail key={selected.queryHash} query={selected} deviceKey={deviceKey} />
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                Select a query to inspect and act on it.
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
