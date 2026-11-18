import { describe, expect, it } from "vitest";

import type { NetworkRequest, NetworkSummary } from "@/backend";

import {
  categoryOf,
  formatBytes,
  formatDuration,
  queryParams,
  requestName,
  statusLabel,
  toCurl,
} from "./presentation";

const row = (overrides: Partial<NetworkSummary> = {}): NetworkSummary => ({
  seq: 1,
  epoch: 1,
  version: 1,
  requestId: "1",
  url: "https://httpbin.org/json",
  method: "GET",
  resourceType: "Fetch",
  state: "complete",
  status: 200,
  statusText: "OK",
  mimeType: "application/json",
  startedAt: 0,
  startTime: 10,
  responseTime: 10.1,
  endTime: 10.25,
  encodedSize: 0,
  decodedSize: 0,
  ...overrides,
});

const request = (overrides: Partial<NetworkRequest> = {}): NetworkRequest => ({
  ...row(),
  requestHeaders: {},
  responseHeaders: {},
  hasPostData: false,
  ...overrides,
});

describe("formatting", () => {
  it("formats sizes in binary units", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(5 * 1024 * 1024)).toBe("5.0 MB");
  });

  it("formats durations and pending requests", () => {
    expect(formatDuration(row())).toBe("250 ms");
    expect(formatDuration(row({ startTime: 1, endTime: 3.456 }))).toBe("2.46 s");
    expect(formatDuration(row({ state: "pending", endTime: 0 }))).toBe("Pending");
  });

  it("labels the status of failed, canceled and pending requests", () => {
    expect(statusLabel(row())).toBe("200");
    expect(statusLabel(row({ state: "pending", status: 0 }))).toBe("Pending");
    expect(statusLabel(row({ state: "failed", status: 0 }))).toBe("(failed)");
    expect(statusLabel(row({ state: "failed", status: 0, canceled: true }))).toBe("(canceled)");
  });

  it("names a request by its last path segment and query", () => {
    expect(requestName("https://httpbin.org/json")).toBe("json");
    expect(requestName("https://api.example.com/v1/todos/?page=2")).toBe("todos/?page=2");
    expect(requestName("https://example.com/")).toBe("example.com");
    expect(requestName("not a url")).toBe("not a url");
  });

  it("decodes query parameters", () => {
    expect(queryParams("https://x/a?b=1&c=x%20y&flag")).toEqual([
      ["b", "1"],
      ["c", "x y"],
      ["flag", ""],
    ]);
    expect(queryParams("https://x/a")).toEqual([]);
  });
});

describe("categoryOf", () => {
  it.each([
    [row({ resourceType: "XHR" }), "fetch"],
    [row({ resourceType: "Fetch" }), "fetch"],
    [row({ resourceType: "Image" }), "img"],
    [row({ resourceType: "Other", mimeType: "image/png" }), "img"],
    [row({ resourceType: "", mimeType: "video/mp4" }), "media"],
    [row({ resourceType: "Script" }), "js"],
    [row({ resourceType: "Document" }), "doc"],
    [row({ resourceType: "WebSocket" }), "ws"],
    [row({ resourceType: "Other", mimeType: "application/octet-stream" }), "other"],
  ])("classifies %#", (r, want) => {
    expect(categoryOf(r)).toBe(want);
  });
});

describe("toCurl", () => {
  it("writes a plain GET with its headers", () => {
    expect(toCurl(request({ requestHeaders: { Accept: "application/json" } }))).toBe(
      "curl 'https://httpbin.org/json' \\\n  -H 'Accept: application/json'",
    );
  });

  it("adds the method and body and quotes single quotes for POSIX shells", () => {
    const curl = toCurl(
      request({
        method: "PUT",
        url: "https://x/it's",
        requestHeaders: {
          "Content-Type": "application/json",
          ":authority": "x",
          "Content-Length": "9",
        },
      }),
      `{"a":"b'c"}`,
    );
    expect(curl).toBe(
      "curl 'https://x/it'\\''s' \\\n  -X 'PUT' \\\n  -H 'Content-Type: application/json' \\\n  --data-raw '{\"a\":\"b'\\''c\"}'",
    );
  });

  it("quotes the method so a hostile one cannot run a second command", () => {
    expect(toCurl(request({ method: "GET;touch /tmp/pwned" }))).toContain(
      "-X 'GET;TOUCH /TMP/PWNED'",
    );
  });

  it("keeps POST implicit when a body is present", () => {
    expect(toCurl(request({ method: "POST" }), "a=1")).toBe(
      "curl 'https://httpbin.org/json' \\\n  --data-raw 'a=1'",
    );
  });
});
