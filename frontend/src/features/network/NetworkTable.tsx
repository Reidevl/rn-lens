import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef, type KeyboardEvent } from "react";

import type { NetworkSummary } from "@/backend";
import { cn } from "@/shared/lib/cn";

import {
  formatBytes,
  formatDuration,
  formatMillis,
  requestName,
  sizeOf,
  statusLabel,
  statusTone,
} from "./presentation";

const COLUMNS = "grid-cols-[minmax(10rem,1fr)_4rem_5rem_5rem_5rem_5rem_minmax(6rem,10rem)]";
const ROW_HEIGHT = 24;

/** Start and end of the waterfall axis, in CDP monotonic seconds. */
function axis(rows: NetworkSummary[]): { start: number; span: number } {
  if (rows.length === 0) return { start: 0, span: 1 };
  let start = Infinity;
  let end = -Infinity;
  for (const r of rows) {
    start = Math.min(start, r.startTime);
    end = Math.max(end, r.endTime || r.responseTime || r.startTime);
  }
  return { start, span: Math.max(end - start, 0.001) };
}

/** Virtualized request table; the waterfall shares one axis across rows. */
export function NetworkTable({
  rows,
  selectedSeq,
  onSelect,
}: {
  rows: NetworkSummary[];
  selectedSeq: number | null;
  onSelect: (seq: number) => void;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  // TanStack Virtual keeps its own mutable instance, which the React Compiler
  // cannot memoize; the component is small enough to skip compilation.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
    getItemKey: (index) => rows[index].seq,
  });
  const { start, span } = axis(rows);
  // One tab stop: the selected row, or the first one.
  const focusIndex = Math.max(
    0,
    rows.findIndex((r) => r.seq === selectedSeq),
  );

  const move = (e: KeyboardEvent, index: number) => {
    const last = rows.length - 1;
    const target =
      e.key === "ArrowDown"
        ? Math.min(index + 1, last)
        : e.key === "ArrowUp"
          ? Math.max(index - 1, 0)
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? last
              : -1;
    if (target < 0) {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onSelect(rows[index].seq);
      }
      return;
    }
    e.preventDefault();
    virtualizer.scrollToIndex(target);
    parentRef.current?.querySelector<HTMLElement>(`[data-seq="${rows[target].seq}"]`)?.focus();
    onSelect(rows[target].seq);
  };

  return (
    <div
      role="grid"
      aria-label="Requests"
      aria-rowcount={rows.length + 1}
      className="flex min-h-0 flex-1 flex-col text-xs"
    >
      <div role="rowgroup" className="border-b border-border bg-card">
        <div
          role="row"
          aria-rowindex={1}
          className={cn("grid gap-2 px-3 py-1 text-muted-foreground", COLUMNS)}
        >
          {["Name", "Method", "Status", "Type", "Size", "Time", "Waterfall"].map((c) => (
            <span key={c} role="columnheader" className="truncate font-medium">
              {c}
            </span>
          ))}
        </div>
      </div>
      <div ref={parentRef} role="rowgroup" className="min-h-0 flex-1 overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((item) => {
            const r = rows[item.index];
            const end = r.endTime || r.responseTime || r.startTime;
            const left = ((r.startTime - start) / span) * 100;
            const width = Math.max(((end - r.startTime) / span) * 100, 0.5);
            const selected = r.seq === selectedSeq;
            return (
              <div
                key={item.key}
                role="row"
                data-seq={r.seq}
                aria-rowindex={item.index + 2}
                aria-selected={selected}
                tabIndex={item.index === focusIndex ? 0 : -1}
                onClick={() => onSelect(r.seq)}
                onKeyDown={(e) => move(e, item.index)}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${item.start}px)`,
                }}
                className={cn(
                  "grid h-6 cursor-pointer items-center gap-2 border-b border-border/40 px-3 outline-none focus-visible:bg-muted",
                  COLUMNS,
                  selected ? "bg-muted" : "hover:bg-muted/50",
                  r.state === "failed" && "text-destructive",
                )}
              >
                <span role="gridcell" className="truncate font-mono" title={r.url}>
                  {requestName(r.url)}
                </span>
                <span role="gridcell">{r.method}</span>
                <span role="gridcell" className={cn("tabular-nums", statusTone(r))}>
                  {statusLabel(r)}
                </span>
                <span role="gridcell" className="truncate text-muted-foreground">
                  {r.resourceType || "Other"}
                </span>
                <span role="gridcell" className="tabular-nums text-muted-foreground">
                  {r.fromCache ? "(cache)" : formatBytes(sizeOf(r))}
                </span>
                <span role="gridcell" className="tabular-nums text-muted-foreground">
                  {formatDuration(r)}
                </span>
                <span role="gridcell" className="relative h-2">
                  <span className="sr-only">
                    {`Starts ${formatMillis((r.startTime - start) * 1000)} after the first request`}
                  </span>
                  <span
                    aria-hidden="true"
                    className={cn(
                      "absolute top-0 h-2 rounded-sm",
                      r.state === "failed"
                        ? "bg-destructive/70"
                        : r.state === "pending"
                          ? "bg-muted-foreground/50"
                          : "bg-accent/70",
                    )}
                    style={{ left: `${left}%`, width: `${Math.min(width, 100 - left)}%` }}
                  />
                </span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
