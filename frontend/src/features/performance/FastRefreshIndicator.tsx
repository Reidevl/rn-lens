import { cn } from "@/shared/lib/cn";
import { useNow } from "@/shared/lib/useNow";

import { usePerformanceStore } from "./performanceStore";
import { presentFastRefresh } from "./presentation";
import { usePerformanceSync } from "./usePerformanceSync";

const toneDot = {
  neutral: "bg-muted-foreground",
  ok: "bg-accent",
  warn: "bg-warning",
  bad: "bg-destructive",
  info: "bg-info",
} as const;

/** Status bar item: the last Fast Refresh of the attached app. */
export function FastRefreshIndicator() {
  usePerformanceSync();
  const state = usePerformanceStore((s) => s.status.fastRefresh);
  const now = useNow(5_000);
  const view = presentFastRefresh(state, now);
  if (!view) return null;
  return (
    <span className="flex items-center gap-1.5" title={view.title}>
      <span aria-hidden="true" className={cn("size-2 rounded-full", toneDot[view.tone])} />
      {view.label}
    </span>
  );
}
