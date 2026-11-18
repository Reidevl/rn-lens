import { Download, Search, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { useBackend } from "@/backend";
import { errorMessage } from "@/features/console/consoleStore";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";

import { useNetworkStore, visibleRows } from "./networkStore";
import { NetworkTable } from "./NetworkTable";
import { CATEGORIES } from "./presentation";
import { RequestDetail } from "./RequestDetail";
import { useNetworkSync } from "./useNetworkSync";

export function NetworkPanel() {
  const backend = useBackend();
  useNetworkSync();
  const rows = useNetworkStore((s) => s.rows);
  const epoch = useNetworkStore((s) => s.epoch);
  const dropped = useNetworkStore((s) => s.dropped);
  const availability = useNetworkStore((s) => s.availability);
  const filter = useNetworkStore((s) => s.filter);
  const setFilter = useNetworkStore((s) => s.setFilter);
  const categories = useNetworkStore((s) => s.categories);
  const toggleCategory = useNetworkStore((s) => s.toggleCategory);
  const clearCategories = useNetworkStore((s) => s.clearCategories);
  const preserveLog = useNetworkStore((s) => s.preserveLog);
  const setPreserveLog = useNetworkStore((s) => s.setPreserveLog);
  const selectedSeq = useNetworkStore((s) => s.selectedSeq);
  const select = useNetworkStore((s) => s.select);
  const busy = useNetworkStore((s) => s.busy);
  const error = useNetworkStore((s) => s.error);
  const notice = useNetworkStore((s) => s.notice);

  const visible = useMemo(
    () => visibleRows(rows, { epoch, preserveLog, categories, filter }),
    [rows, epoch, preserveLog, categories, filter],
  );
  const selected = visible.find((r) => r.seq === selectedSeq) ?? null;

  const run = async (action: () => Promise<void>) => {
    const store = useNetworkStore.getState();
    store.setBusy(true);
    store.setError(null);
    store.setNotice(null);
    try {
      await action();
    } catch (err) {
      useNetworkStore.getState().setError(errorMessage(err));
    } finally {
      useNetworkStore.getState().setBusy(false);
    }
  };
  const exportHAR = () =>
    run(async () => {
      const path = await backend.network.saveHAR();
      if (path) useNetworkStore.getState().setNotice(`Saved ${path}`);
    });

  return (
    <section aria-labelledby="panel-network-title" className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-2 border-b border-border px-3">
        <h1 id="panel-network-title" className="text-sm font-semibold">
          Network
        </h1>
        <label className="relative ml-2 flex max-w-xs min-w-0 flex-1 items-center">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute left-2 size-3.5 text-muted-foreground"
          />
          <input
            type="search"
            aria-label="Filter requests by URL"
            placeholder="Filter by URL"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 w-full rounded-md border border-border bg-background pr-2 pl-7 text-sm outline-none focus-visible:outline-2 focus-visible:outline-ring"
          />
        </label>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={preserveLog}
            onChange={(e) => setPreserveLog(e.target.checked)}
          />
          Preserve log
        </label>
        <div className="flex-1" />
        <Button size="sm" disabled={busy || rows.length === 0} onClick={() => void exportHAR()}>
          <Download aria-hidden="true" className="size-3.5" />
          Export HAR
        </Button>
        <Button size="sm" disabled={busy} onClick={() => void run(() => backend.network.clear())}>
          <Trash2 aria-hidden="true" className="size-3.5" />
          Clear
        </Button>
      </header>

      <div
        role="group"
        aria-label="Type filters"
        className="flex flex-wrap gap-1 border-b border-border px-3 py-1.5"
      >
        <Chip active={categories.length === 0} onClick={clearCategories}>
          All
        </Chip>
        {CATEGORIES.map((c) => (
          <Chip key={c.id} active={categories.includes(c.id)} onClick={() => toggleCategory(c.id)}>
            {c.label}
          </Chip>
        ))}
      </div>

      {error ? (
        <p
          role="alert"
          className="border-b border-destructive/60 bg-destructive/10 px-3 py-1 text-xs"
        >
          {error}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="border-b border-border bg-muted/40 px-3 py-1 text-xs">
          {notice}
        </p>
      ) : null}

      {!availability.checked && rows.length === 0 ? (
        <Empty>Attach to an app on the Targets panel to inspect its network traffic.</Empty>
      ) : availability.checked && !availability.supported && rows.length === 0 ? (
        <Empty>
          <span className="font-medium text-foreground">{availability.reason}</span>
          <span>Requests appear here when the app runs React Native 0.83 or newer.</span>
        </Empty>
      ) : (
        <div
          className={cn(
            "grid min-h-0 flex-1",
            selected ? "grid-cols-[minmax(20rem,1fr)_minmax(22rem,1fr)]" : "grid-cols-1",
          )}
        >
          <div className="flex min-h-0 flex-col">
            <NetworkTable rows={visible} selectedSeq={selectedSeq} onSelect={select} />
            <footer className="border-t border-border px-3 py-1 text-xs text-muted-foreground">
              {visible.length} of {rows.length} requests
              {dropped > 0 ? ` · ${dropped} older requests dropped` : ""}
              {rows.length === 0 ? " · Requests the app makes appear here." : ""}
            </footer>
          </div>
          {selected ? <RequestDetail row={selected} onClose={() => select(null)} /> : null}
        </div>
      )}
    </section>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: string;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "h-6 cursor-pointer rounded-md px-2 text-xs transition-colors duration-150",
        active ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted/60",
      )}
    >
      {children}
    </button>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-1 p-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}
