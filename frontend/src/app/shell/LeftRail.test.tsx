import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LeftRail } from "./LeftRail";
import { hasInsetTitleBar } from "./titleBar";

const mac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15";

describe("hasInsetTitleBar", () => {
  it("is true only in the macOS desktop app", () => {
    expect(hasInsetTitleBar({ _wails: {} }, mac)).toBe(true);
    expect(hasInsetTitleBar({}, mac)).toBe(false); // browser mode
    expect(hasInsetTitleBar({ _wails: {} }, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")).toBe(
      false,
    );
  });
});

describe("LeftRail", () => {
  afterEach(() => {
    delete (window as { _wails?: unknown })._wails;
  });

  it("leaves the macOS window buttons alone in a draggable title bar, like Finder", () => {
    (window as { _wails?: unknown })._wails = {};
    Object.defineProperty(navigator, "userAgent", { value: mac, configurable: true });
    const { container } = render(<LeftRail />);
    const bar = container.querySelector('[data-titlebar="inset"]') as HTMLElement;
    expect(bar).not.toBeNull();
    expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
    expect(bar.textContent).toBe("");
    expect(screen.queryByText("Lens")).not.toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Panels" })).toBeInTheDocument();
  });

  it("keeps the plain header elsewhere", () => {
    render(<LeftRail />);
    const header = screen.getByText("Lens").parentElement!;
    expect(header).not.toHaveAttribute("data-titlebar");
  });
});
