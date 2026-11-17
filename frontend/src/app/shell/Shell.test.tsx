import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { createMockBackend } from "@/backend";

import { PANELS } from "./panels";
import { useShellStore } from "./shellStore";

describe("Shell", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "targets" });
    document.documentElement.dataset.theme = "";
    window.localStorage.clear();
  });

  it("lists every panel in the left rail and shows the build version", async () => {
    render(<App backend={createMockBackend({ info: { version: "9.9.9-test" } })} />);

    for (const panel of PANELS) {
      expect(screen.getByRole("button", { name: panel.label })).toBeInTheDocument();
    }
    expect(await screen.findByText("Lens v9.9.9-test")).toBeInTheDocument();
  });

  it("switches the active panel from the rail", async () => {
    const user = userEvent.setup();
    render(<App backend={createMockBackend()} />);

    await user.click(screen.getByRole("button", { name: "Console" }));

    expect(screen.getByRole("button", { name: "Console" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("heading", { name: "Console" })).toBeInTheDocument();
  });

  it("toggles between dark and light themes", async () => {
    const user = userEvent.setup();
    render(<App backend={createMockBackend()} />);

    await user.click(screen.getByRole("button", { name: /switch to light theme/i }));

    expect(document.documentElement.dataset.theme).toBe("light");
  });
});
