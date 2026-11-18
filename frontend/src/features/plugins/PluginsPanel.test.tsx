import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, type MockBackend, type PluginDevice, type PluginInfo } from "@/backend";

import { usePluginsStore } from "./pluginsStore";

const device: PluginDevice = {
  key: "dev.rnlens.example|d1",
  sessionId: "s1",
  app: { id: "dev.rnlens.example", name: "Lens Example", platform: "ios" },
  device: { id: "d1", name: "iPhone" },
  sdkVersion: "0.1.0",
  connectedAt: "2026-11-18T12:00:00Z",
};

const flags: PluginInfo = {
  id: "feature-flags",
  name: "Feature flags",
  description: "Remote config overrides",
  blocks: [
    { type: "text", id: "intro", text: "Overrides last until restart.", tone: "muted" },
    {
      type: "table",
      id: "flags",
      title: "Flags",
      columns: [
        { key: "name", label: "Flag" },
        { key: "on", label: "On" },
      ],
      rows: [
        { id: "darkMode", cells: { name: "darkMode", on: false } },
        { id: "newCheckout", cells: { name: "newCheckout", on: true } },
      ],
      rowActions: [{ id: "toggle", label: "Toggle" }],
    },
    {
      type: "keyValue",
      id: "config",
      title: "Config",
      items: [{ key: "env", label: "Environment", value: "staging" }],
    },
    { type: "json", id: "raw", title: "Raw", value: { ttl: 300 } },
    {
      type: "log",
      id: "events",
      title: "Events",
      entries: [{ level: "warn", text: "darkMode overridden" }],
    },
    {
      type: "actions",
      id: "tools",
      actions: [
        { id: "refetch", label: "Refetch config" },
        {
          id: "reset",
          label: "Reset overrides",
          destructive: true,
          confirm: "Reset every override?",
        },
      ],
    },
  ],
};

function withPlugins(...plugins: PluginInfo[]): MockBackend {
  const backend = createMockBackend();
  backend.setPlugins(device, plugins);
  return backend;
}

describe("PluginsPanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "plugins" });
    usePluginsStore.getState().reset();
  });

  it("explains how to add a panel when no app has one", async () => {
    render(<App backend={createMockBackend()} />);
    expect(await screen.findByText("No app has added a panel to Lens yet.")).toBeInTheDocument();
    expect(screen.getByText(/createLensPanel/)).toBeInTheDocument();
  });

  it("draws every block kind of the plugin's view", async () => {
    render(<App backend={withPlugins(flags)} />);
    const panel = await screen.findByRole("tabpanel");
    expect(within(panel).getByText("Remote config overrides")).toBeInTheDocument();
    expect(within(panel).getByText("Overrides last until restart.")).toBeInTheDocument();
    const table = within(panel).getByRole("table", { name: "Flags" });
    expect(within(table).getAllByRole("row")).toHaveLength(3);
    expect(within(table).getByText("darkMode")).toBeInTheDocument();
    expect(within(panel).getByText("Environment")).toBeInTheDocument();
    expect(within(panel).getByText("staging")).toBeInTheDocument();
    expect(within(panel).getByRole("tree", { name: "Raw" })).toBeInTheDocument();
    expect(within(panel).getByText("darkMode overridden")).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Refetch config" })).toBeInTheDocument();
  });

  it("sends a row action and shows the app's message", async () => {
    const user = userEvent.setup();
    const backend = withPlugins(flags);
    backend.onPluginAction(({ rowId }) => `${rowId} is on`);
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("button", { name: "Toggle darkMode" }));
    expect(await screen.findByText("darkMode is on")).toBeInTheDocument();
    expect(backend.calls).toContain("plugins.action:feature-flags/flags/toggle/darkMode");
  });

  it("asks before a confirmed action and can cancel", async () => {
    const user = userEvent.setup();
    const backend = withPlugins(flags);
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("button", { name: "Reset overrides" }));
    expect(screen.getByText("Reset every override?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(backend.calls.some((c) => c.startsWith("plugins.action"))).toBe(false);

    await user.click(screen.getByRole("button", { name: "Reset overrides" }));
    await user.click(screen.getByRole("button", { name: "Confirm Reset overrides" }));
    expect(backend.calls).toContain("plugins.action:feature-flags/tools/reset");
  });

  it("shows a failing action as an error", async () => {
    const user = userEvent.setup();
    const backend = withPlugins(flags);
    backend.onPluginAction(() => {
      throw new Error("plugins: action failed in the app: config server down");
    });
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("button", { name: "Refetch config" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("config server down");
  });

  it("explains blocks it cannot draw and views Lens refused", async () => {
    const odd: PluginInfo = {
      id: "odd",
      name: "Odd",
      blocks: [{ type: "chart", id: "c" } as never],
    };
    const big: PluginInfo = {
      id: "big",
      name: "Big",
      blocks: [],
      error: "The plugin's view is larger than 1 MiB.",
    };
    const user = userEvent.setup();
    render(<App backend={withPlugins(odd, big)} />);
    expect(
      await screen.findByText("This block (chart) needs a newer version of Lens."),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Big" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("larger than 1 MiB");
  });

  it("follows a new view and keeps the selected plugin", async () => {
    const user = userEvent.setup();
    const other: PluginInfo = {
      id: "other",
      name: "Other",
      blocks: [{ type: "text", id: "t", text: "one" }],
    };
    const backend = withPlugins(flags, other);
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("tab", { name: "Other" }));
    expect(await screen.findByText("one")).toBeInTheDocument();
    act(() =>
      backend.setPlugins(device, [
        flags,
        { ...other, blocks: [{ type: "text", id: "t", text: "two" }] },
      ]),
    );
    expect(await screen.findByText("two")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Other" })).toHaveAttribute("aria-selected", "true");
  });
});

describe("PluginsPanel robustness", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "plugins" });
    usePluginsStore.getState().reset();
  });

  it("keeps the panel when one block is malformed", async () => {
    const broken: PluginInfo = {
      id: "broken",
      name: "Broken",
      blocks: [
        { type: "table", id: "t", title: "Bad table" } as never, // no columns or rows
        { type: "text", id: "ok", text: "Still here" },
      ],
    };
    render(<App backend={withPlugins(broken)} />);
    expect(await screen.findByText("Still here")).toBeInTheDocument();
    expect(screen.getByText(/This block could not be drawn/)).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Broken" })).toBeInTheDocument();
  });

  it("cancels a confirmation with Escape", async () => {
    const user = userEvent.setup();
    const backend = withPlugins(flags);
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("button", { name: "Reset overrides" }));
    expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByText("Reset every override?")).not.toBeInTheDocument();
    expect(backend.calls.some((c) => c.startsWith("plugins.action"))).toBe(false);
  });
});
