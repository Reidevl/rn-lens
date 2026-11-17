import { beforeEach, describe, expect, it } from "vitest";

import { createMockBackend } from "@/backend";

import { pullConsole, useConsoleStore } from "./consoleStore";

const seqs = () => useConsoleStore.getState().entries.map((e) => e.seq);

describe("pullConsole", () => {
  beforeEach(() => {
    useConsoleStore.getState().reset();
  });

  it("does not repeat entries that landed between Stats and Range", async () => {
    const backend = createMockBackend();
    backend.log({ text: "1" });
    // Make the first Stats call stale: entries 2 and 3 appear right after it.
    const realStats = backend.console.stats;
    backend.console.stats = async () => {
      const stats = await realStats();
      backend.log({ text: "2" }, { text: "3" });
      backend.console.stats = realStats;
      return stats;
    };

    await pullConsole(backend);
    await pullConsole(backend);

    expect(seqs()).toEqual([1, 2, 3]);
  });

  it("empties the list after a Go-side clear even when an append follows it", async () => {
    const backend = createMockBackend();
    backend.log({ text: "a" }, { text: "b" });
    await pullConsole(backend);
    expect(seqs()).toEqual([1, 2]);

    await backend.console.clear();
    backend.log({ text: "c" });
    await pullConsole(backend);

    expect(useConsoleStore.getState().entries.map((e) => e.text)).toEqual(["c"]);
  });

  it("skips entries Go already dropped and reports the gap", async () => {
    const backend = createMockBackend({ consoleCapacity: 2 });
    backend.log({ text: "1" }, { text: "2" }, { text: "3" });

    await pullConsole(backend);

    expect(seqs()).toEqual([2, 3]);
    expect(useConsoleStore.getState().dropped).toBe(1);
  });
});
