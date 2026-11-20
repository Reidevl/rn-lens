import { render, screen } from "@testing-library/react";
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
});
