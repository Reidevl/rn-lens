import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useNow } from "./useNow";

describe("useNow", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("re-renders with the current time on every tick and stops on unmount", () => {
    const { result, unmount } = renderHook(() => useNow(1_000));
    const first = result.current;
    expect(first).toBe(Date.parse("2026-01-01T00:00:00Z"));

    act(() => {
      vi.advanceTimersByTime(2_500);
    });
    expect(result.current).toBe(first + 2_000);

    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
