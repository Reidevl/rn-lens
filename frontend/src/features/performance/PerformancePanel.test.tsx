import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, idleSession, sampleTarget, type MockBackend } from "@/backend";

import { idlePerformance, usePerformanceStore } from "./performanceStore";

function attachedBackend(): MockBackend {
  const target = sampleTarget();
  return createMockBackend({
    targets: [target],
    session: { ...idleSession, state: "attached", target },
  });
}

describe("PerformancePanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "performance" });
    usePerformanceStore.setState({ status: idlePerformance });
  });

  it("asks to attach first", async () => {
    render(<App backend={createMockBackend()} />);
    expect(
      await screen.findByText("Attach to an app on the Targets panel to record its performance."),
    ).toBeInTheDocument();
  });

  it("captures, previews and saves a screenshot", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    render(<App backend={backend} />);
    const card = await screen.findByRole("region", { name: "Screenshot" });

    await user.click(within(card).getByRole("button", { name: "Capture" }));
    expect(
      await within(card).findByRole("img", { name: "Screenshot of the attached app" }),
    ).toHaveAttribute("src", expect.stringMatching(/^data:image\/png;base64,/));
    await user.click(within(card).getByRole("button", { name: "Save…" }));
    expect(await within(card).findByText("/Users/me/lens-screenshot.png")).toBeInTheDocument();
    expect(backend.calls).toEqual(
      expect.arrayContaining(["performance.screenshot", "performance.saveScreenshot"]),
    );
  });

  it("records a trace with the chosen categories and shows its summary", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    render(<App backend={backend} />);
    const card = await screen.findByRole("region", { name: "Trace" });

    // Defaults come from the backend; Screenshots is opt-in.
    expect(await within(card).findByRole("checkbox", { name: "Timeline" })).toBeChecked();
    expect(within(card).getByRole("checkbox", { name: "Screenshots" })).not.toBeChecked();
    await user.click(within(card).getByRole("checkbox", { name: "Screenshots" }));

    await user.click(within(card).getByRole("button", { name: "Start recording" }));
    expect(backend.calls).toContain(
      "performance.startTrace:disabled-by-default-v8.cpu_profiler,devtools.timeline,disabled-by-default-devtools.screenshot",
    );
    await user.click(await within(card).findByRole("button", { name: "Stop" }));
    expect(await within(card).findByText(/4\.2 s · 1,520 events/)).toBeInTheDocument();

    await user.click(within(card).getByRole("button", { name: "Save trace…" }));
    expect(await within(card).findByText("/Users/me/lens-trace.json")).toBeInTheDocument();
  });

  it("profiles the CPU and reports errors in the card", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    render(<App backend={backend} />);
    const card = await screen.findByRole("region", { name: "CPU profile" });

    await user.click(within(card).getByRole("button", { name: "Start profiling" }));
    await user.click(await within(card).findByRole("button", { name: "Stop" }));
    expect(await within(card).findByText(/3,900 samples/)).toBeInTheDocument();

    backend.failPerformance("cdp: Profiler.start failed");
    await user.click(within(card).getByRole("button", { name: "Start profiling" }));
    expect(await within(card).findByRole("alert")).toHaveTextContent("cdp: Profiler.start failed");
  });

  it("saves a heap snapshot", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    render(<App backend={backend} />);
    const card = await screen.findByRole("region", { name: "Heap snapshot" });
    await user.click(within(card).getByRole("button", { name: "Take snapshot…" }));
    expect(await within(card).findByText(/\/Users\/me\/lens\.heapsnapshot/)).toBeInTheDocument();
  });

  it("shows the last Fast Refresh in the status bar", async () => {
    const backend = attachedBackend();
    render(<App backend={backend} />);
    act(() =>
      backend.setPerformance({
        fastRefresh: { phase: "error", message: "SyntaxError: App.tsx: Unexpected token" },
      }),
    );
    const item = await screen.findByText("Fast Refresh failed");
    expect(item).toHaveAttribute("title", "SyntaxError: App.tsx: Unexpected token");
  });
});
