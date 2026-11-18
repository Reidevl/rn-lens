import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { createMockBackend } from "@/backend";

import { mcpCommand } from "./command";

describe("mcpCommand", () => {
  it("builds the claude mcp add command, with the token header when one is set", () => {
    expect(mcpCommand({ url: "http://127.0.0.1:7431/mcp" })).toBe(
      "claude mcp add --transport http lens http://127.0.0.1:7431/mcp",
    );
    expect(mcpCommand({ url: "http://127.0.0.1:7431/mcp", token: "s3cret" })).toBe(
      'claude mcp add --transport http lens http://127.0.0.1:7431/mcp --header "Authorization: Bearer s3cret"',
    );
  });
});

describe("MCP status bar item", () => {
  it("copies the connect command", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend({ mcp: { url: "http://127.0.0.1:7431/mcp" } });
    render(<App backend={backend} />);
    const button = await screen.findByRole("button", {
      name: /Copy the command that connects Claude/,
    });
    expect(button).toHaveAttribute("title", expect.stringContaining("http://127.0.0.1:7431/mcp"));
    await user.click(button);
    expect(backend.clipboardText).toBe(
      "claude mcp add --transport http lens http://127.0.0.1:7431/mcp",
    );
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("is hidden when MCP is off", async () => {
    render(<App backend={createMockBackend()} />);
    await screen.findByText(/Lens v/);
    expect(screen.queryByRole("button", { name: /connects Claude/ })).not.toBeInTheDocument();
  });
});
