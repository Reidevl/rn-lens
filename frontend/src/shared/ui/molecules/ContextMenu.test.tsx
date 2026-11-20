import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ContextMenu } from "./ContextMenu";

describe("ContextMenu", () => {
  it("focuses the first enabled item, moves with the arrows and closes on an outside press", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const pick = vi.fn();
    render(
      <>
        <p>outside</p>
        <ContextMenu
          x={10}
          y={10}
          label="Actions"
          onClose={onClose}
          items={[
            { label: "Off", onSelect: () => {}, disabled: true },
            { label: "One", onSelect: () => {} },
            { label: "Two", onSelect: pick },
          ]}
        />
      </>,
    );
    expect(screen.getByRole("menuitem", { name: "One" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Two" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "One" })).toHaveFocus();
    await user.keyboard("{ArrowUp}{Enter}");
    expect(pick).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();

    await user.click(screen.getByText("outside"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("closes on Escape with no focusable item and on Tab", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const { unmount } = render(
      <ContextMenu
        x={0}
        y={0}
        label="Actions"
        onClose={onClose}
        items={[{ label: "Off", onSelect: () => {}, disabled: true }]}
      />,
    );
    expect(screen.getByRole("menu")).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
    unmount();

    const onTab = vi.fn();
    render(
      <ContextMenu
        x={0}
        y={0}
        label="Actions"
        onClose={onTab}
        items={[{ label: "One", onSelect: () => {} }]}
      />,
    );
    await user.keyboard("{Tab}");
    expect(onTab).toHaveBeenCalled();
  });

  it("stays open on programmatic scroll and closes on the mouse wheel", () => {
    const onClose = vi.fn();
    render(
      <>
        <div data-testid="log" />
        <ContextMenu
          x={0}
          y={0}
          label="Actions"
          onClose={onClose}
          items={[{ label: "One", onSelect: () => {} }]}
        />
      </>,
    );
    fireEvent.scroll(screen.getByTestId("log"));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.wheel(screen.getByTestId("log"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("gives focus back to where it was when it closes", async () => {
    const user = userEvent.setup();
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            open
          </button>
          {open ? (
            <ContextMenu
              x={0}
              y={0}
              label="Actions"
              onClose={() => setOpen(false)}
              items={[{ label: "One", onSelect: () => {} }]}
            />
          ) : null}
        </>
      );
    }
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "open" }));
    expect(screen.getByRole("menuitem", { name: "One" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "open" })).toHaveFocus();
  });
});
