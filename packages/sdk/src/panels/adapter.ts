import type { LensMessage, PluginActionMessage, PluginBlock } from "@rnlens/protocol";

import { jsonByteLength, toSafeJson } from "../serialize";
import type { AdapterHost } from "../tanstack/adapter";
import type { LensPanel } from "./panel";

/** Lens shows an error instead of a plugin.view larger than this. */
export const PANEL_VIEW_MAX_BYTES = 1024 * 1024;

/** Refreshes within this window send one view. */
const REFRESH_COALESCE_MS = 16;

export interface PanelsAdapter {
  attach(): void;
  detach(): void;
  sendSnapshot(): void;
  handle(message: LensMessage): boolean;
}

const errorBlock = (text: string): PluginBlock[] => [
  { type: "text", id: "lens-error", text, tone: "error" },
];

export function createPanelsAdapter(input: LensPanel[], host: AdapterHost): PanelsAdapter {
  const panels = new Map<string, LensPanel>();
  for (const panel of input) {
    if (panels.has(panel.id)) {
      host.log.warn(`two panels use the id "${panel.id}"; Lens shows the first`);
      continue;
    }
    panels.set(panel.id, panel);
  }
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  /** The latest render started per panel; an older one finishing later is dropped. */
  const renders = new Map<string, number>();
  let unsubscribes: Array<() => void> = [];

  async function sendView(panel: LensPanel): Promise<void> {
    const ticket = (renders.get(panel.id) ?? 0) + 1;
    renders.set(panel.id, ticket);
    let blocks: PluginBlock[];
    try {
      blocks = toSafeJson(await panel.render()) as PluginBlock[];
    } catch (err) {
      blocks = errorBlock(
        `The panel failed to render: ${err instanceof Error ? err.message : String(err)}`,
      );
    }
    if (renders.get(panel.id) !== ticket) return; // a newer render already ran
    let frame = JSON.stringify({
      v: 1,
      type: "plugin.view",
      payload: { pluginId: panel.id, blocks },
    });
    if (jsonByteLength(frame) > PANEL_VIEW_MAX_BYTES) {
      blocks = errorBlock(
        `The panel's view is larger than 1 MiB (${Math.ceil(jsonByteLength(frame) / 1024)} KiB): show fewer rows or shorter values.`,
      );
      frame = JSON.stringify({
        v: 1,
        type: "plugin.view",
        payload: { pluginId: panel.id, blocks },
      });
    }
    if (host.isReady()) host.sendRaw(frame);
  }

  function schedule(panel: LensPanel): void {
    if (!host.isReady() || timers.has(panel.id)) return;
    timers.set(
      panel.id,
      setTimeout(() => {
        timers.delete(panel.id);
        void sendView(panel);
      }, REFRESH_COALESCE_MS),
    );
  }

  async function runAction(message: PluginActionMessage): Promise<void> {
    const { id, pluginId, blockId, actionId, rowId } = message.payload;
    const panel = panels.get(pluginId);
    if (!panel) {
      host.send({
        v: 1,
        type: "plugin.result",
        id,
        payload: { id, ok: false, error: `no plugin "${pluginId}" in this app` },
      });
      return;
    }
    try {
      const result = await panel.onAction?.({ blockId, actionId, ...(rowId ? { rowId } : {}) });
      const payload =
        typeof result === "string" && result !== ""
          ? { id, ok: true, message: result.slice(0, 280) }
          : { id, ok: true };
      host.send({ v: 1, type: "plugin.result", id, payload });
    } catch (err) {
      host.send({
        v: 1,
        type: "plugin.result",
        id,
        payload: { id, ok: false, error: err instanceof Error ? err.message : String(err) },
      });
    }
    await sendView(panel);
  }

  return {
    attach() {
      unsubscribes = [...panels.values()].map((panel) => panel.subscribe(() => schedule(panel)));
    },
    detach() {
      for (const off of unsubscribes) off();
      unsubscribes = [];
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
    },
    sendSnapshot() {
      for (const panel of panels.values()) {
        const { id, name, description, version } = panel;
        host.send({
          v: 1,
          type: "plugin.register",
          payload: {
            id,
            name,
            ...(description ? { description } : {}),
            ...(version ? { version } : {}),
          },
        });
        void sendView(panel);
      }
    },
    handle(message) {
      if (message.type !== "plugin.action") return false;
      void runAction(message);
      return true;
    },
  };
}
