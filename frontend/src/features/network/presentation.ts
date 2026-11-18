import type { NetworkRequest, NetworkSummary } from "@/backend";

export type Category = "fetch" | "doc" | "js" | "img" | "media" | "font" | "ws" | "other";

/** Type filter chips, in display order. */
export const CATEGORIES: Array<{ id: Category; label: string }> = [
  { id: "fetch", label: "Fetch/XHR" },
  { id: "doc", label: "Doc" },
  { id: "js", label: "JS" },
  { id: "img", label: "Img" },
  { id: "media", label: "Media" },
  { id: "font", label: "Font" },
  { id: "ws", label: "WS" },
  { id: "other", label: "Other" },
];

const BY_RESOURCE_TYPE: Record<string, Category> = {
  fetch: "fetch",
  xhr: "fetch",
  document: "doc",
  script: "js",
  image: "img",
  media: "media",
  font: "font",
  websocket: "ws",
};

/** Filter category from the CDP resource type, falling back to the MIME type. */
export function categoryOf(r: Pick<NetworkSummary, "resourceType" | "mimeType">): Category {
  const byType = BY_RESOURCE_TYPE[r.resourceType.toLowerCase()];
  if (byType) return byType;
  const mime = r.mimeType.toLowerCase();
  if (mime.startsWith("image/")) return "img";
  if (mime.startsWith("video/") || mime.startsWith("audio/")) return "media";
  if (mime.startsWith("font/")) return "font";
  return "other";
}

export type SortKey = "name" | "method" | "status" | "type" | "size" | "time" | "start";
export type SortDir = "asc" | "desc";
export interface Sort {
  key: SortKey;
  dir: SortDir;
}

/** Newest request first, like a feed: what the app just did is on top. */
export const DEFAULT_SORT: Sort = { key: "start", dir: "desc" };

/** Failed and pending requests sort after every status code. */
const statusRank = (r: NetworkSummary) =>
  r.state === "failed" ? 1000 : r.status === 0 ? 999 : r.status;
const duration = (r: NetworkSummary) =>
  r.endTime > 0 ? r.endTime - r.startTime : Number.POSITIVE_INFINITY;

const VALUE: Record<SortKey, (r: NetworkSummary) => number | string> = {
  name: (r) => requestName(r.url).toLowerCase(),
  method: (r) => r.method,
  status: statusRank,
  type: (r) => (r.resourceType || "Other").toLowerCase(),
  size: (r) => sizeOf(r),
  time: duration,
  start: (r) => r.startTime,
};

const SORT_NAMES: Record<SortKey, string> = {
  name: "name",
  method: "method",
  status: "status",
  type: "type",
  size: "size",
  time: "time",
  start: "start time",
};

/** The order in words, for the footer (its column may be hidden). */
export function sortLabel(sort: Sort): string {
  if (sort.key === "start") return sort.dir === "desc" ? "Newest first" : "Oldest first";
  return `Sorted by ${SORT_NAMES[sort.key]}, ${sort.dir === "asc" ? "ascending" : "descending"}`;
}

/** Rows ordered by the sort; ties fall back to arrival order (seq) in the same direction. */
export function sortRows(rows: NetworkSummary[], sort: Sort): NetworkSummary[] {
  const value = VALUE[sort.key];
  const sign = sort.dir === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    const va = value(a);
    const vb = value(b);
    const cmp = va < vb ? -1 : va > vb ? 1 : 0;
    return cmp !== 0 ? cmp * sign : (a.seq - b.seq) * sign;
  });
}

export { formatBytes } from "@/shared/lib/formatBytes";

/** Bytes the row shows: on the wire when known, otherwise decoded. */
export function sizeOf(r: NetworkSummary): number {
  return r.encodedSize > 0 ? r.encodedSize : r.decodedSize;
}

export function formatMillis(ms: number): string {
  return ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(2)} s`;
}

export function formatDuration(r: NetworkSummary): string {
  if (r.endTime <= 0) return r.state === "pending" ? "Pending" : "";
  return formatMillis((r.endTime - r.startTime) * 1000);
}

export function statusLabel(r: NetworkSummary): string {
  if (r.state === "failed") return r.canceled ? "(canceled)" : "(failed)";
  if (r.status === 0) return r.state === "pending" ? "Pending" : "";
  return String(r.status);
}

/** Tailwind text color for the status cell. */
export function statusTone(r: NetworkSummary): string {
  if (r.state === "failed" || r.status >= 400) return "text-destructive";
  if (r.status >= 300) return "text-warning";
  if (r.state === "pending") return "text-muted-foreground";
  return "";
}

/** Short name: the last path segment plus the query, or the host for "/". */
export function requestName(url: string): string {
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return url;
  }
  const segments = u.pathname.split("/");
  let last = segments.pop() ?? "";
  if (last === "" && segments.length > 1) last = `${segments.pop() ?? ""}/`;
  return (last === "/" || last === "" ? u.host : last) + u.search;
}

export function queryParams(url: string): Array<[string, string]> {
  try {
    return [...new URL(url).searchParams.entries()];
  } catch {
    return [];
  }
}

export function isJSONMime(mime: string): boolean {
  return /[/+]json\b/i.test(mime);
}

export function isImageMime(mime: string): boolean {
  return mime.toLowerCase().startsWith("image/");
}

const shellQuote = (s: string) => `'${s.replaceAll("'", "'\\''")}'`;

/** POSIX shell cURL command equivalent to the request. */
export function toCurl(r: NetworkRequest, body = ""): string {
  const parts = [`curl ${shellQuote(r.url)}`];
  const method = r.method.toUpperCase();
  const implicit = body ? method === "POST" : method === "GET";
  if (!implicit) parts.push(`-X ${shellQuote(method)}`);
  for (const [name, value] of Object.entries(r.requestHeaders ?? {})) {
    // HTTP/2 pseudo headers and the length are recomputed by curl.
    if (name.startsWith(":") || name.toLowerCase() === "content-length") continue;
    parts.push(`-H ${shellQuote(`${name}: ${value}`)}`);
  }
  if (body) parts.push(`--data-raw ${shellQuote(body)}`);
  return parts.join(" \\\n  ");
}
