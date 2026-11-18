import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { PanelErrorBoundary } from "./PanelErrorBoundary";

let broken = true;
function Flaky() {
  if (broken) throw new Error("Objects are not valid as a React child");
  return <p>Panel is back</p>;
}

describe("PanelErrorBoundary", () => {
  it("shows the error in place of the panel and recovers on Try again", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const user = userEvent.setup();
    render(
      <PanelErrorBoundary panel="Storage">
        <Flaky />
      </PanelErrorBoundary>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("The Storage panel hit an error");
    expect(screen.getByText(/Objects are not valid as a React child/)).toBeInTheDocument();

    broken = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(screen.getByText("Panel is back")).toBeInTheDocument();
  });
});
