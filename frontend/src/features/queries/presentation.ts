import type { QueryLabel } from "@/backend";

export const labelText: Record<QueryLabel, string> = {
  fetching: "Fetching",
  fresh: "Fresh",
  stale: "Stale",
  inactive: "Inactive",
  paused: "Paused",
};

/** Dot colours follow the official devtools: blue, green, yellow, gray, purple. */
export const labelDot: Record<QueryLabel, string> = {
  fetching: "bg-info",
  fresh: "bg-accent",
  stale: "bg-warning",
  inactive: "bg-muted-foreground",
  paused: "bg-paused",
};

export function keyText(key: unknown): string {
  try {
    return JSON.stringify(key);
  } catch {
    return String(key);
  }
}

/** Relative label for `ms` measured from `now`, so callers can drive it from a ticking clock. */
export function ago(ms: number, now: number = Date.now()): string {
  if (!ms) return "never";
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  return `${Math.round(s / 3600)}h ago`;
}

export function statusBadge(status: string): string {
  switch (status) {
    case "error":
      return "bg-destructive/15 text-destructive";
    case "pending":
      return "bg-warning/15 text-warning";
    case "success":
      return "bg-accent/15 text-accent";
    default:
      return "bg-muted text-muted-foreground";
  }
}
