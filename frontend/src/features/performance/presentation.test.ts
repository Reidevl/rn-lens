import { describe, expect, it } from "vitest";

import { formatDuration, presentFastRefresh } from "./presentation";

describe("formatDuration", () => {
  it("picks a unit", () => {
    expect(formatDuration(850.4)).toBe("850 ms");
    expect(formatDuration(4210)).toBe("4.2 s");
    expect(formatDuration(65_000)).toBe("1 min 5 s");
  });
});

describe("presentFastRefresh", () => {
  const now = Date.parse("2026-11-18T12:00:30Z");

  it("hides the indicator while detached", () => {
    expect(presentFastRefresh({ phase: "off" }, now)).toBeNull();
  });

  it("reports the last update with its module count and age", () => {
    expect(
      presentFastRefresh({ phase: "updated", modules: 1, at: "2026-11-18T12:00:28Z" }, now),
    ).toEqual({ label: "Fast Refresh: 1 module just now", tone: "ok" });
    expect(
      presentFastRefresh({ phase: "updated", modules: 3, at: "2026-11-18T12:00:00Z" }, now)?.label,
    ).toBe("Fast Refresh: 3 modules 30 s ago");
  });

  it("hides a phase it does not know, from a newer Lens core", () => {
    expect(presentFastRefresh({ phase: "paused" as never }, now)).toBeNull();
  });

  it("explains failures in the tooltip", () => {
    expect(presentFastRefresh({ phase: "error", message: "SyntaxError: App.tsx" }, now)).toEqual({
      label: "Fast Refresh failed",
      tone: "bad",
      title: "SyntaxError: App.tsx",
    });
  });
});
