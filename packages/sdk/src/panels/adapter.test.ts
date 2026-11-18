import type { PluginBlock, PluginResultPayload } from "@rnlens/protocol";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createLensClient, type LensClient } from "../client";
import { FakeLens } from "../testing/fake-lens";
import { PANEL_VIEW_MAX_BYTES } from "./adapter";
import { createLensPanel, type LensPanel } from "./panel";

let lens: FakeLens;
let client: LensClient | undefined;
let nextId = 0;

beforeEach(async () => {
  lens = await FakeLens.start();
});

afterEach(async () => {
  client?.disconnect();
  client = undefined;
  await lens.close();
  expect(lens.invalid).toEqual([]);
});

async function connect(...panels: LensPanel[]) {
  client = createLensClient({
    url: lens.url,
    enabled: true,
    panels,
    backoff: { minMs: 20, maxMs: 40 },
  });
  client.connect();
  return lens.welcome();
}

async function click(
  pluginId: string,
  blockId: string,
  actionId: string,
  rowId?: string,
): Promise<PluginResultPayload> {
  const id = `p${++nextId}`;
  lens.send({
    v: 1,
    type: "plugin.action",
    id,
    payload: { id, pluginId, blockId, actionId, ...(rowId ? { rowId } : {}) },
  });
  const result = await lens.next("plugin.result");
  expect(result.payload.id).toBe(id);
  return result.payload;
}

function flagsPanel() {
  const flags: Record<string, boolean> = { newCheckout: true, darkMode: false };
  const seen: unknown[] = [];
  const panel = createLensPanel({
    id: "feature-flags",
    name: "Feature flags",
    description: "Remote config overrides",
    render: (): PluginBlock[] => [
      {
        type: "table",
        id: "flags",
        columns: [
          { key: "name", label: "Flag" },
          { key: "on", label: "On" },
        ],
        rows: Object.entries(flags).map(([name, on]) => ({ id: name, cells: { name, on } })),
        rowActions: [{ id: "toggle", label: "Toggle" }],
      },
      { type: "actions", id: "tools", actions: [{ id: "boom", label: "Boom" }] },
    ],
    onAction: (action) => {
      seen.push(action);
      if (action.actionId === "boom") throw new Error("no such thing");
      if (action.rowId) flags[action.rowId] = !flags[action.rowId];
      return `${action.rowId} is ${flags[action.rowId!] ? "on" : "off"}`;
    },
  });
  return { panel, flags, seen };
}

describe("panels", () => {
  it("announces the plugins feature, then registers each panel and sends its view", async () => {
    const { panel } = flagsPanel();
    const hello = await connect(panel);
    expect(hello.payload.features).toContain("plugins");
    const reg = await lens.next("plugin.register");
    expect(reg.payload).toEqual({
      id: "feature-flags",
      name: "Feature flags",
      description: "Remote config overrides",
    });
    const view = await lens.next("plugin.view");
    expect(view.payload.pluginId).toBe("feature-flags");
    expect(view.payload.blocks.map((b) => b.type)).toEqual(["table", "actions"]);
  });

  it("runs actions with their row and answers with the message, then re-renders", async () => {
    const { panel, seen } = flagsPanel();
    await connect(panel);
    await lens.next("plugin.view");
    expect(await click("feature-flags", "flags", "toggle", "darkMode")).toEqual({
      id: `p${nextId}`,
      ok: true,
      message: "darkMode is on",
    });
    expect(seen).toEqual([{ blockId: "flags", actionId: "toggle", rowId: "darkMode" }]);
    const view = await lens.next("plugin.view");
    const table = view.payload.blocks[0] as Extract<PluginBlock, { type: "table" }>;
    expect(table.rows.find((r) => r.id === "darkMode")?.cells.on).toBe(true);
  });

  it("reports a failing action and an unknown plugin", async () => {
    const { panel } = flagsPanel();
    await connect(panel);
    await lens.next("plugin.view");
    expect(await click("feature-flags", "tools", "boom")).toMatchObject({
      ok: false,
      error: "no such thing",
    });
    expect(await click("other", "b", "a")).toMatchObject({
      ok: false,
      error: 'no plugin "other" in this app',
    });
  });

  it("sends one view for several refreshes in a row", async () => {
    let n = 0;
    const panel = createLensPanel({
      id: "counter",
      name: "Counter",
      render: () => [{ type: "text", id: "n", text: String(n) }],
    });
    await connect(panel);
    await lens.next("plugin.view");
    n = 3;
    panel.refresh();
    panel.refresh();
    panel.refresh();
    const view = await lens.next("plugin.view");
    expect(view.payload.blocks).toEqual([{ type: "text", id: "n", text: "3" }]);
    await lens.sync();
    expect(lens.drain("plugin.view")).toEqual([]);
  });

  it("shows render failures and oversized views as an error block", async () => {
    let mode: "throw" | "huge" | "ok" = "throw";
    const panel = createLensPanel({
      id: "fragile",
      name: "Fragile",
      render: () => {
        if (mode === "throw") throw new Error("state not ready");
        if (mode === "huge")
          return [
            { type: "text", id: "big", text: "x".repeat(4000) },
            ...Array.from({ length: 99 }, (_, i) => ({
              type: "json" as const,
              id: `j${i}`,
              value: "y".repeat(PANEL_VIEW_MAX_BYTES / 90),
            })),
          ];
        return [{ type: "text", id: "ok", text: "fine" }];
      },
    });
    await connect(panel);
    let view = await lens.next("plugin.view");
    expect(view.payload.blocks).toEqual([
      {
        type: "text",
        id: "lens-error",
        text: "The panel failed to render: state not ready",
        tone: "error",
      },
    ]);
    mode = "huge";
    panel.refresh();
    view = await lens.next("plugin.view");
    expect(view.payload.blocks[0]).toMatchObject({ id: "lens-error", tone: "error" });
    expect((view.payload.blocks[0] as { text: string }).text).toMatch(/larger than 1 MiB/);
    mode = "ok";
    panel.refresh();
    view = await lens.next("plugin.view");
    expect(view.payload.blocks).toEqual([{ type: "text", id: "ok", text: "fine" }]);
  });

  it("sends values JSON cannot carry in a safe form", async () => {
    const panel = createLensPanel({
      id: "odd",
      name: "Odd values",
      render: () => [
        {
          type: "json",
          id: "v",
          value: { big: 10n, when: new Date("2026-11-18T12:00:00Z"), gone: undefined } as never,
        },
      ],
    });
    await connect(panel);
    const view = await lens.next("plugin.view");
    expect(view.payload.blocks[0]).toEqual({
      type: "json",
      id: "v",
      value: { big: "10", when: "2026-11-18T12:00:00.000Z" },
    });
  });

  it("refuses invalid panel ids and duplicate panels", async () => {
    expect(() => createLensPanel({ id: "Feature Flags", name: "x", render: () => [] })).toThrow(
      /invalid panel id/,
    );
    const a = createLensPanel({ id: "same", name: "A", render: () => [] });
    const b = createLensPanel({ id: "same", name: "B", render: () => [] });
    await connect(a, b);
    expect((await lens.next("plugin.register")).payload.name).toBe("A");
    await lens.next("plugin.view");
    await lens.sync();
    expect(lens.drain("plugin.register")).toEqual([]);
  });
});
