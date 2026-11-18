import { describe, expect, it, vi } from "vitest";

import { detectMode, fileName, startDownload, uploadFile } from "./browserFiles";

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
    ...init,
  });

describe("detectMode", () => {
  it("reports browser mode only when Lens says so", async () => {
    expect(await detectMode(async () => json({ mode: "browser" }))).toBe("browser");
    expect(await detectMode(async () => json({ mode: "desktop" }))).toBeNull();
  });

  it("ignores the Vite dev server's HTML fallback and network errors", async () => {
    const html = async () =>
      new Response("<!doctype html>", { headers: { "content-type": "text/html" } });
    expect(await detectMode(html)).toBeNull();
    expect(
      await detectMode(async () => {
        throw new TypeError("offline");
      }),
    ).toBeNull();
  });
});

describe("files", () => {
  it("names a download after the path's last segment", () => {
    expect(fileName("/tmp/lens-transfers-1/abc/lens-trace.json")).toBe("lens-trace.json");
    expect(fileName("C:\\Temp\\x\\lens.har")).toBe("lens.har");
  });

  it("downloads through a temporary link", () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    startDownload("/lens/downloads/t/lens.har", "lens.har");
    const a = click.mock.contexts[0] as HTMLAnchorElement;
    expect(a.getAttribute("href")).toBe("/lens/downloads/t/lens.har");
    expect(a.download).toBe("lens.har");
    expect(document.querySelector("a[download]")).toBeNull();
    click.mockRestore();
  });

  it("uploads and returns the token, or Lens's error", async () => {
    const calls: [string, RequestInit | undefined][] = [];
    const token = await uploadFile(new Blob(["{}"]), async (url, init) => {
      calls.push([String(url), init]);
      return json({ token: "t1" });
    });
    expect(token).toBe("t1");
    expect(calls[0][0]).toBe("/lens/uploads");
    expect(calls[0][1]?.method).toBe("POST");
    await expect(
      uploadFile(
        new Blob(["x"]),
        async () => new Response("the file must be at most 64 MiB\n", { status: 413 }),
      ),
    ).rejects.toThrow("the file must be at most 64 MiB");
  });
});
