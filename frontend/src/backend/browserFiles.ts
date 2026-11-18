/**
 * Browser mode (the rnlens server build) has no native file dialogs: Go
 * writes a save to a one-time download URL, and an import starts with an
 * upload. These helpers are the browser half.
 */

/** Asks Lens which shell runs it; "browser" only under the server build. */
export async function detectMode(fetchImpl: typeof fetch = fetch): Promise<"browser" | null> {
  try {
    const res = await fetchImpl("/lens/mode", { cache: "no-store" });
    if (!res.ok || !res.headers.get("content-type")?.includes("application/json")) return null;
    const body = (await res.json()) as { mode?: string };
    return body.mode === "browser" ? "browser" : null;
  } catch {
    return null;
  }
}

/** The last path segment: what a download is called. */
export function fileName(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

/** Starts a browser download of url. */
export function startDownload(url: string, name: string): void {
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.rel = "noopener";
  document.body.append(a);
  a.click();
  a.remove();
}

/** Opens the browser's file picker; resolves null when the user cancels. */
export function pickFile(accept: string): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = accept;
    input.addEventListener("change", () => resolve(input.files?.[0] ?? null), { once: true });
    input.addEventListener("cancel", () => resolve(null), { once: true });
    input.click();
  });
}

/** Uploads a file to Lens and returns its one-time token. */
export async function uploadFile(file: Blob, fetchImpl: typeof fetch = fetch): Promise<string> {
  const res = await fetchImpl("/lens/uploads", { method: "POST", body: file });
  if (!res.ok) throw new Error((await res.text()).trim() || `upload failed (${res.status})`);
  const { token } = (await res.json()) as { token?: string };
  if (!token) throw new Error("upload failed: no token");
  return token;
}
