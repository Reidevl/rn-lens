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
