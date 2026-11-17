import type { ConsoleEntry, ConsoleLevel, RemoteObject } from "@/backend";

/** HH:MM:SS.mmm in local time. */
export function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}

export const levelLabel: Record<ConsoleLevel, string> = {
  verbose: "Verbose",
  info: "Info",
  warning: "Warnings",
  error: "Errors",
  system: "System",
};

/** Row background and text classes per entry. */
export function rowClasses(entry: ConsoleEntry): string {
  switch (entry.level) {
    case "error":
      return "bg-destructive/10 text-destructive";
    case "warning":
      return "bg-warning/10 text-warning";
    case "verbose":
      return "text-muted-foreground";
    case "system":
      return entry.kind === "system" ? "text-muted-foreground" : "text-foreground";
    default:
      return "text-foreground";
  }
}

/** Objects that can be expanded: have a handle and are not null. */
export function isExpandable(o: RemoteObject): boolean {
  return (o.type === "object" || o.type === "function") && !!o.objectId && o.subtype !== "null";
}

/** Shortens a bundle URL to its path for stack frames. */
export function shortUrl(url: string): string {
  try {
    const u = new URL(url);
    return u.pathname.replace(/\/\/.*$/, "");
  } catch {
    return url;
  }
}
