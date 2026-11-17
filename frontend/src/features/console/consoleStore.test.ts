import { beforeEach, describe, expect, it } from "vitest";

import { createMockBackend } from "@/backend";

import { pullConsole, runConsoleAction, useConsoleStore } from "./consoleStore";

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

  it("pages through the missing tail until it reaches Stats.lastSeq", async () => {
    const backend = createMockBackend({ consoleCapacity: 5_000 });
    const total = 4_500;
    backend.log(...Array.from({ length: total }, (_, i) => ({ text: `line ${i + 1}` })));

    await pullConsole(backend);

    const state = useConsoleStore.getState();
    expect(state.entries).toHaveLength(total);
    expect(state.entries[0].seq).toBe(1);
    expect(state.entries[total - 1].seq).toBe(total);
    expect(state.lastSeq).toBe(total);
    expect(backend.calls.filter((c) => c.startsWith("console.range:"))).toEqual([
      "console.range:1:2000",
      "console.range:2001:2000",
      "console.range:4001:2000",
    ]);
  });

  it("skips entries Go already dropped and reports the gap", async () => {
    const backend = createMockBackend({ consoleCapacity: 2 });
    backend.log({ text: "1" }, { text: "2" }, { text: "3" });

    await pullConsole(backend);

    expect(seqs()).toEqual([2, 3]);
    expect(useConsoleStore.getState().dropped).toBe(1);
  });
});

describe("runConsoleAction", () => {
  beforeEach(() => {
    useConsoleStore.getState().reset();
  });

  it("stores the failure message and releases busy", async () => {
    await runConsoleAction(() => Promise.reject(new Error("evaluate: session closed")));

    expect(useConsoleStore.getState().error).toBe("evaluate: session closed");
    expect(useConsoleStore.getState().busy).toBe(false);
  });

  it("clears a previous failure when the next action succeeds", async () => {
    useConsoleStore.getState().setError("stale");

    await runConsoleAction(() => Promise.resolve());

    expect(useConsoleStore.getState().error).toBeNull();
    expect(useConsoleStore.getState().busy).toBe(false);
  });
});

describe("mock console backend", () => {
  it("owns seq and epoch even when a caller passes them", () => {
    const backend = createMockBackend();

    const [entry] = backend.log({ text: "x", seq: 99, epoch: 7 });

    expect(entry.seq).toBe(1);
    expect(entry.epoch).toBe(1);
  });

  it("reports evaluate results as RemoteObjects of the value's own type", async () => {
    const backend = createMockBackend();

    await expect(backend.console.evaluate("1+1")).resolves.toMatchObject({
      kind: "result",
      text: "2",
      args: [{ type: "number", value: 2 }],
    });
    await expect(backend.console.evaluate('"hi"')).resolves.toMatchObject({
      kind: "result",
      text: "hi",
      args: [{ type: "string", value: "hi" }],
    });
    await expect(backend.console.evaluate("null")).resolves.toMatchObject({
      kind: "result",
      text: "null",
      args: [{ type: "object", subtype: "null" }],
    });
    await expect(backend.console.evaluate('{"a":1}')).resolves.toMatchObject({
      kind: "result",
      args: [{ type: "object", className: "Object" }],
    });
  });

  it("answers unknown expressions with an exception instead of executing them", async () => {
    const backend = createMockBackend();

    await expect(backend.console.evaluate("window.secret")).resolves.toMatchObject({
      kind: "exception",
      level: "error",
    });
    await expect(backend.console.evaluate("throw")).resolves.toMatchObject({
      kind: "exception",
      text: "Error: thrown from mock",
    });
  });
});
