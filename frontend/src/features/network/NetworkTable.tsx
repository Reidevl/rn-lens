import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown, ArrowUp } from "lucide-react";
import { useRef, type KeyboardEvent, type ReactNode } from "react";

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
  type Sort,
  type SortKey,
} from "./presentation";

const ROW_HEIGHT = 24;

interface Column {
  id: string;
  label: string;
  sortKey: SortKey;
  /** CSS grid track. */
  width: string;
  /** Kept when the detail pane is open. */
  compact?: boolean;
  cell: (r: NetworkSummary, axis: Axis) => ReactNode;
  className?: (r: NetworkSummary) => string;
}

interface Axis {
  start: number;
  span: number;
}

const COLUMNS: Column[] = [
  {
    id: "name",
    label: "Name",
    sortKey: "name",
    width: "minmax(8rem,1fr)",
    compact: true,
    cell: (r) => (
      <span className="block truncate font-mono" title={r.url}>
        {requestName(r.url)}
      </span>
    ),
  },
  { id: "method", label: "Method", sortKey: "method", width: "4rem", cell: (r) => r.method },
  {
    id: "status",
    label: "Status",
    sortKey: "status",
    width: "5rem",
    compact: true,
    cell: (r) => statusLabel(r),
    className: (r) => cn("tabular-nums", statusTone(r)),
  },
  {
    id: "type",
    label: "Type",
    sortKey: "type",
    width: "5rem",
    cell: (r) => r.resourceType || "Other",
    className: () => "truncate text-muted-foreground",
  },
  {
    id: "size",
    label: "Size",
    sortKey: "size",
    width: "5rem",
    cell: (r) => (r.fromCache ? "(cache)" : formatBytes(sizeOf(r))),
    className: () => "tabular-nums text-muted-foreground",
  },
  {
    id: "time",
    label: "Time",
    sortKey: "time",
    width: "5rem",
    compact: true,
    cell: (r) => formatDuration(r),
    className: () => "tabular-nums text-muted-foreground",
  },
  {
    id: "waterfall",
    label: "Waterfall",
    sortKey: "start",
    width: "minmax(6rem,10rem)",
    cell: (r, { start, span }) => {
      const end = r.endTime || r.responseTime || r.startTime;
      const left = ((r.startTime - start) / span) * 100;
      const width = Math.max(((end - r.startTime) / span) * 100, 0.5);
      return (
        <>
          <span className="sr-only">
            {`Starts ${formatMillis((r.startTime - start) * 1000)} after the first request`}
          </span>
          <span
            aria-hidden="true"
            className={cn(
              "absolute top-1/2 h-2 -translate-y-1/2 rounded-sm",
              r.state === "failed"
                ? "bg-destructive/70"
                : r.state === "pending"
                  ? "bg-muted-foreground/50"
                  : "bg-accent/70",
            )}
            style={{ left: `${left}%`, width: `${Math.min(width, 100 - left)}%` }}
          />
        </>
      );
    },
    className: () => "relative h-full",
  },
];

/** Start and end of the waterfall axis, in CDP monotonic seconds. */
function axisOf(rows: NetworkSummary[]): Axis {
  if (rows.length === 0) return { start: 0, span: 1 };
  let start = Infinity;
  let end = -Infinity;
  for (const r of rows) {
    start = Math.min(start, r.startTime);
    end = Math.max(end, r.endTime || r.responseTime || r.startTime);
  }
  return { start, span: Math.max(end - start, 0.001) };
}

/**
 * Virtualized request grid. Headers sort (the Waterfall header sorts by start
 * time); `compact` keeps Name, Status and Time while the detail pane is open.
 */
export function NetworkTable({
  rows,
  selectedSeq,
  onSelect,
  sort,
  onSort,
  compact = false,
}: {
  rows: NetworkSummary[];
  selectedSeq: number | null;
  onSelect: (seq: number) => void;
  sort: Sort;
  onSort: (key: SortKey) => void;
  compact?: boolean;
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
  const axis = axisOf(rows);
  const columns = compact ? COLUMNS.filter((c) => c.compact) : COLUMNS;
  const template = { gridTemplateColumns: columns.map((c) => c.width).join(" ") };
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
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden text-xs"
    >
      <div role="rowgroup" className="border-b border-border bg-card">
        <div
          role="row"
          aria-rowindex={1}
          style={template}
          className="grid gap-2 px-3 py-1 text-muted-foreground"
        >
          {columns.map((c) => {
            const active = sort.key === c.sortKey;
            return (
              <span
                key={c.id}
                role="columnheader"
                aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}
                className="min-w-0"
              >
                <button
                  type="button"
                  aria-label={`Sort by ${c.label}`}
                  onClick={() => onSort(c.sortKey)}
                  className={cn(
                    "flex w-full min-w-0 cursor-pointer items-center gap-1 rounded-sm text-left font-medium hover:text-foreground",
                    active && "text-foreground",
                  )}
                >
                  <span className="truncate">{c.label}</span>
                  {active ? (
                    sort.dir === "asc" ? (
                      <ArrowUp aria-hidden="true" className="size-3 shrink-0" />
                    ) : (
                      <ArrowDown aria-hidden="true" className="size-3 shrink-0" />
                    )
                  ) : null}
                </button>
              </span>
            );
          })}
        </div>
      </div>
      <div ref={parentRef} role="rowgroup" className="min-h-0 flex-1 overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((item) => {
            const r = rows[item.index];
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
                  ...template,
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  transform: `translateY(${item.start}px)`,
                }}
                className={cn(
                  "grid h-6 cursor-pointer items-center gap-2 border-b border-border/40 px-3 outline-none focus-visible:bg-muted",
                  selected ? "bg-muted" : "hover:bg-muted/50",
                  r.state === "failed" && "text-destructive",
                )}
              >
                {columns.map((c) => (
                  <span key={c.id} role="gridcell" className={cn("min-w-0", c.className?.(r))}>
                    {c.cell(r, axis)}
                  </span>
                ))}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
