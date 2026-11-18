import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend } from "@/backend";

import { useMcpStore } from "./mcpStore";

const url = "http://127.0.0.1:7431/mcp";

describe("MCP panel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "mcp" });
    useMcpStore.getState().reset();
  });

  it("sits at the bottom of the sidebar, apart from the app's panels", async () => {
    render(<App backend={createMockBackend({ mcp: { url } })} />);
    const nav = await screen.findByRole("navigation", { name: "Panels" });
    const lists = within(nav).getAllByRole("list");
    expect(lists).toHaveLength(2);
    expect(within(lists[1]).getByRole("button", { name: "MCP" })).toBeInTheDocument();
    expect(within(lists[0]).queryByRole("button", { name: "MCP" })).not.toBeInTheDocument();
  });

  it("explains how to turn MCP on when it is off", async () => {
    render(<App backend={createMockBackend()} />);
    expect(await screen.findByText(/MCP is off/)).toBeInTheDocument();
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  it("copies the connect snippets", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend({ mcp: { url, token: "s3cret" } });
    render(<App backend={backend} />);
    const connect = await screen.findByRole("region", { name: "Connect" });
    expect(within(connect).getByText(url)).toBeInTheDocument();
    await user.click(within(connect).getByRole("button", { name: "Copy the Claude Code command" }));
    expect(backend.clipboardText).toBe(
      `claude mcp add --transport http lens ${url} --header 'Authorization: Bearer s3cret'`,
    );
    await user.click(
      within(connect).getByRole("button", { name: "Copy the Cursor configuration" }),
    );
    expect(JSON.parse(backend.clipboardText)).toEqual({
      mcpServers: { lens: { url, headers: { Authorization: "Bearer s3cret" } } },
    });
    expect(connect.textContent).not.toContain("s3cret");
  });

  it("switches read-only mode", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend({ mcp: { url } });
    render(<App backend={backend} />);
    const toggle = await screen.findByRole("switch", { name: /Read-only/ });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    await user.click(toggle);
    expect(backend.calls).toContain("mcp.setReadOnly:true");
    expect(await screen.findByRole("switch", { name: /Read-only/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("lists the tools with their kind", async () => {
    render(<App backend={createMockBackend({ mcp: { url } })} />);
    const tools = await screen.findByRole("region", { name: "Tools" });
    expect(await within(tools).findByText("clear_storage")).toBeInTheDocument();
    expect(within(tools).getByText("destructive")).toBeInTheDocument();
    expect(within(tools).getByText("change")).toBeInTheDocument();
  });

  it("shows the activity live, newest first, and clears it", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend({ mcp: { url } });
    render(<App backend={backend} />);
    const activity = await screen.findByRole("region", { name: "Activity" });
    expect(within(activity).getByText(/No AI tool calls yet/)).toBeInTheDocument();
    act(() => {
      backend.recordMcpCall({ tool: "list_queries", result: "[]" });
      backend.recordMcpCall({
        tool: "clear_storage",
        kind: "destructive",
        args: { store: "async-storage" },
        error: "Lens is read-only for AI clients",
        refused: true,
      });
    });
    await within(activity).findByRole("button", { name: /clear_storage/ });
    const rows = within(activity).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("clear_storage");
    expect(rows[1]).toHaveTextContent("refused");
    expect(rows[2]).toHaveTextContent("list_queries");
    await user.click(within(rows[1]).getByRole("button", { name: /clear_storage/ }));
    expect(await within(activity).findByText(/"store": "async-storage"/)).toBeInTheDocument();
    await user.click(within(activity).getByRole("button", { name: "Clear" }));
    expect(await within(activity).findByText(/No AI tool calls yet/)).toBeInTheDocument();
  });
});
