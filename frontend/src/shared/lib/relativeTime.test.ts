import { describe, expect, it } from "vitest";

import { relativeTime } from "./relativeTime";

describe("relativeTime", () => {
  const now = Date.parse("2026-01-01T00:10:00Z");

  it("formats the distance from the given clock, not from Date.now()", () => {
    expect(relativeTime(now - 5_000, now)).toBe("5s ago");
    expect(relativeTime(now - 120_000, now)).toBe("2m ago");
    expect(relativeTime(now - 7_200_000, now)).toBe("2h ago");
  });

  it("reports never for a missing timestamp and clamps the future to zero", () => {
    expect(relativeTime(0, now)).toBe("never");
    expect(relativeTime(now + 60_000, now)).toBe("0s ago");
  });
});
