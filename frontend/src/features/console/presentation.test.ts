import { describe, expect, it } from "vitest";

import { shortUrl } from "./presentation";

describe("shortUrl", () => {
  it("keeps only the bundle path of a Metro URL whose query is encoded as //&", () => {
    expect(shortUrl("http://localhost:8081/index.bundle//&platform=ios&dev=true")).toBe(
      "/index.bundle",
    );
  });

  it("drops a regular query string", () => {
    expect(shortUrl("http://localhost:8081/index.bundle?platform=android")).toBe("/index.bundle");
  });

  it("returns a non-URL unchanged", () => {
    expect(shortUrl("native")).toBe("native");
  });
});
