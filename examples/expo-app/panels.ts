import { createLensPanel, type LensPanelBlock } from "@rnlens/sdk";

/** Feature flags the app could read; Lens toggles them through a panel. */
const defaults = { newCheckout: false, darkMode: false, betaSearch: true };
const flags: Record<string, boolean> = { ...defaults };
const events: { time: string; level: "info" | "warn"; text: string }[] = [];

function record(level: "info" | "warn", text: string) {
  events.push({ time: new Date().toISOString(), level, text });
  if (events.length > 50) events.shift();
}

/** The example's plugin panel: a table with row actions, buttons and a log. */
export const flagsPanel = createLensPanel({
  id: "feature-flags",
  name: "Feature flags",
  description: "Flags of the example app. Toggling one changes it in the app until reload.",
  version: "1.0.0",
  render: (): LensPanelBlock[] => [
    {
      type: "table",
      id: "flags",
      title: "Flags",
      columns: [
        { key: "name", label: "Flag" },
        { key: "on", label: "On" },
        { key: "default", label: "Default" },
      ],
      rows: Object.entries(flags).map(([name, on]) => ({
        id: name,
        cells: { name, on, default: defaults[name as keyof typeof defaults] },
      })),
      rowActions: [{ id: "toggle", label: "Toggle" }],
    },
    {
      type: "keyValue",
      id: "summary",
      title: "Summary",
      items: [
        { key: "on", label: "Flags on", value: Object.values(flags).filter(Boolean).length },
        { key: "changes", label: "Changes", value: events.length },
      ],
    },
    {
      type: "actions",
      id: "tools",
      actions: [
        { id: "reset", label: "Reset to defaults", destructive: true, confirm: "Reset every flag?" },
      ],
    },
    { type: "log", id: "events", title: "Changes", entries: [...events] },
  ],
  onAction: ({ actionId, rowId }) => {
    if (actionId === "toggle" && rowId && rowId in flags) {
      flags[rowId] = !flags[rowId];
      record("info", `${rowId} turned ${flags[rowId] ? "on" : "off"} from Lens`);
      return `${rowId} is ${flags[rowId] ? "on" : "off"}`;
    }
    if (actionId === "reset") {
      Object.assign(flags, defaults);
      record("warn", "Flags reset to defaults from Lens");
      return "Flags reset";
    }
    throw new Error(`unknown action ${actionId}`);
  },
});
