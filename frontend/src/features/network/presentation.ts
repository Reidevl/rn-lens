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

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

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
  if (!implicit) parts.push(`-X ${method}`);
  for (const [name, value] of Object.entries(r.requestHeaders ?? {})) {
    // HTTP/2 pseudo headers and the length are recomputed by curl.
    if (name.startsWith(":") || name.toLowerCase() === "content-length") continue;
    parts.push(`-H ${shellQuote(`${name}: ${value}`)}`);
  }
  if (body) parts.push(`--data-raw ${shellQuote(body)}`);
  return parts.join(" \\\n  ");
}
