import { describe, expect, it } from "vitest";

import { ago } from "./presentation";

describe("ago", () => {
  const now = Date.parse("2026-01-01T00:10:00Z");

  it("formats the distance from the given clock, not from Date.now()", () => {
    expect(ago(now - 5_000, now)).toBe("5s ago");
    expect(ago(now - 120_000, now)).toBe("2m ago");
    expect(ago(now - 7_200_000, now)).toBe("2h ago");
  });

  it("reports never for a missing timestamp and clamps the future to zero", () => {
    expect(ago(0, now)).toBe("never");
    expect(ago(now + 60_000, now)).toBe("0s ago");
  });
});
