import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ConsoleEntry } from "@/backend";

import { ConsoleList } from "./ConsoleList";
import { useConsoleStore } from "./consoleStore";

const scrollToIndex = vi.fn();

// jsdom has no layout; render every row and spy on the scroll request.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 26,
        size: 26,
      })),
    getTotalSize: () => count * 26,
    scrollToIndex,
    measureElement: () => {},
  }),
}));

function entry(seq: number): ConsoleEntry {
  return {
    seq,
    timestamp: "2026-01-01T00:00:00Z",
    kind: "log",
    source: "console",
    level: "info",
    text: `line ${seq}`,
    epoch: 1,
  };
}

/** Gives the log element a scrollable geometry; jsdom reports 0 for everything. */
function setGeometry(el: HTMLElement, scrollTop: number) {
  Object.defineProperty(el, "scrollHeight", { configurable: true, value: 1000 });
  Object.defineProperty(el, "clientHeight", { configurable: true, value: 300 });
  el.scrollTop = scrollTop;
}

describe("ConsoleList", () => {
  beforeEach(() => {
    scrollToIndex.mockClear();
    useConsoleStore.getState().reset();
  });

  it("follows the tail while new entries arrive", () => {
    const { rerender } = render(<ConsoleList entries={[entry(1), entry(2)]} />);
    expect(scrollToIndex).toHaveBeenLastCalledWith(1, { align: "end" });

    rerender(<ConsoleList entries={[entry(1), entry(2), entry(3)]} />);
    expect(scrollToIndex).toHaveBeenLastCalledWith(2, { align: "end" });
    expect(screen.queryByRole("button", { name: /jump to latest/i })).not.toBeInTheDocument();
  });

  it("stops following when the user scrolls up and resumes from the jump button", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ConsoleList entries={[entry(1), entry(2)]} />);
    const log = screen.getByRole("log");

    setGeometry(log, 0);
    fireEvent.scroll(log);
    expect(useConsoleStore.getState().follow).toBe(false);
    scrollToIndex.mockClear();
    rerender(<ConsoleList entries={[entry(1), entry(2), entry(3)]} />);
    expect(scrollToIndex).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: /jump to latest/i }));
    expect(useConsoleStore.getState().follow).toBe(true);
    expect(scrollToIndex).toHaveBeenLastCalledWith(2, { align: "end" });
    expect(screen.queryByRole("button", { name: /jump to latest/i })).not.toBeInTheDocument();
  });

  it("resumes following when the user scrolls back within the threshold", () => {
    render(<ConsoleList entries={[entry(1), entry(2)]} />);
    const log = screen.getByRole("log");

    setGeometry(log, 0);
    fireEvent.scroll(log);
    expect(useConsoleStore.getState().follow).toBe(false);

    setGeometry(log, 695);
    fireEvent.scroll(log);
    expect(useConsoleStore.getState().follow).toBe(true);
  });
});
