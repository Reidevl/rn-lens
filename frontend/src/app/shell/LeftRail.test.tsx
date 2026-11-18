import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LeftRail } from "./LeftRail";
import { hasInsetTitleBar } from "./titleBar";

describe("hasInsetTitleBar", () => {
  it("is true only in the macOS desktop app", () => {
    expect(hasInsetTitleBar({ _wails: { environment: { OS: "darwin" } } })).toBe(true);
    expect(hasInsetTitleBar({ _wails: { environment: { OS: "windows" } } })).toBe(false);
    // Browser mode on a Mac: the runtime created _wails without environment.
    expect(hasInsetTitleBar({ _wails: {} })).toBe(false);
    expect(hasInsetTitleBar({})).toBe(false);
  });
});

describe("LeftRail", () => {
  afterEach(() => {
    delete (window as { _wails?: unknown })._wails;
  });

  it("leaves the macOS window buttons alone in a draggable title bar, like Finder", () => {
    (window as { _wails?: unknown })._wails = { environment: { OS: "darwin" } };
    const { container } = render(<LeftRail />);
    const bar = container.querySelector('[data-titlebar="inset"]') as HTMLElement;
    expect(bar).not.toBeNull();
    expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
    expect(bar.textContent).toBe("");
    expect(screen.queryByText("Lens")).not.toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Panels" })).toBeInTheDocument();
  });

  it("keeps the plain header elsewhere, including browser mode on a Mac", () => {
    (window as { _wails?: unknown })._wails = {};
    render(<LeftRail />);
    const header = screen.getByText("Lens").parentElement!;
    expect(header).not.toHaveAttribute("data-titlebar");
  });
});
