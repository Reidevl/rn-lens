import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, idleSession, sampleTarget, type ConsoleEntry } from "@/backend";
import { useTargetsStore } from "@/features/targets/targetsStore";

import { useConsoleStore } from "./consoleStore";

// jsdom has no layout, so the virtualizer would render nothing. Render every
// row instead; virtualization itself is covered by the library.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 26,
        size: 26,
      })),
    getTotalSize: () => count * 26,
    scrollToIndex: () => {},
    measureElement: () => {},
  }),
}));

function attachedBackend() {
  const target = sampleTarget();
  return createMockBackend({
    targets: [target],
    session: { ...idleSession, state: "attached", target },
  });
}

describe("ConsolePanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "console" });
    useConsoleStore.getState().reset();
    useTargetsStore.setState({ targets: [], session: idleSession, error: null, busy: false });
  });

  it("shows entries as Go appends them and filters by level and text", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    render(<App backend={backend} />);
    expect(await screen.findByText(/Nothing logged yet/)).toBeInTheDocument();

    backend.log(
      { text: "tick 1" },
      { kind: "warning", text: "careful" },
      { kind: "error", text: "boom" },
    );

    const log = await screen.findByRole("log");
    expect(within(log).getByText("tick 1")).toBeInTheDocument();
    expect(within(log).getByText("boom")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^Info/ }));
    expect(within(log).queryByText("tick 1")).not.toBeInTheDocument();
    expect(within(log).getByText("careful")).toBeInTheDocument();

    await user.type(screen.getByRole("searchbox", { name: /filter console/i }), "boom");
    expect(within(log).queryByText("careful")).not.toBeInTheDocument();
    expect(within(log).getByText("boom")).toBeInTheDocument();
  });

  it("clears through the backend", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.log({ text: "old line" });
    render(<App backend={backend} />);
    await screen.findByText("old line");

    await user.click(screen.getByRole("button", { name: /clear console/i }));

    await waitFor(() => expect(backend.calls).toContain("console.clear"));
    expect(await screen.findByText(/Nothing logged yet/)).toBeInTheDocument();
  });

  it("evaluates expressions with history", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    render(<App backend={backend} />);
    const input = await screen.findByRole("textbox", { name: /evaluate expression/i });

    await user.type(input, "1+1{Enter}");

    await waitFor(() => expect(backend.calls).toContain("console.evaluate:1+1"));
    const log = await screen.findByRole("log");
    expect(await within(log).findByText("1+1")).toBeInTheDocument();
    expect(within(log).getByText("2")).toBeInTheDocument();

    await user.keyboard("{ArrowUp}");
    expect(input).toHaveValue("1+1");
  });

  it("keeps the input focused and enabled while an evaluation is in flight", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    const realEvaluate = backend.console.evaluate;
    let release = () => {};
    backend.console.evaluate = (expression) =>
      new Promise<ConsoleEntry>((resolve) => {
        release = () => void realEvaluate(expression).then(resolve);
      });
    render(<App backend={backend} />);
    const input = await screen.findByRole("textbox", { name: /evaluate expression/i });

    await user.type(input, "1+1{Enter}");

    await waitFor(() => expect(useConsoleStore.getState().busy).toBe(true));
    expect(input).toBeEnabled();
    expect(input).toHaveFocus();
    expect(input).toHaveAttribute("aria-busy", "true");

    release();
    await waitFor(() => expect(useConsoleStore.getState().busy).toBe(false));
    expect(input).toHaveAttribute("aria-busy", "false");
    const log = await screen.findByRole("log");
    expect(await within(log).findByText("2")).toBeInTheDocument();
  });

  it("surfaces evaluate and clear failures in the alert banner", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    backend.console.evaluate = () => Promise.reject(new Error("evaluate: session closed"));
    render(<App backend={backend} />);
    const input = await screen.findByRole("textbox", { name: /evaluate expression/i });
    await waitFor(() => expect(input).toBeEnabled());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    await user.type(input, "1+1{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent("evaluate: session closed");

    backend.console.clear = () => Promise.reject(new Error("clear: session closed"));
    await user.click(screen.getByRole("button", { name: /clear console/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent("clear: session closed");
  });

  it("reports a failing sync instead of staying silently empty", async () => {
    const backend = createMockBackend();
    const realStats = backend.console.stats;
    backend.console.stats = () => Promise.reject(new Error("stats: backend unavailable"));
    render(<App backend={backend} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("stats: backend unavailable");

    backend.console.stats = realStats;
    backend.console.range = () => Promise.reject(new Error("range: backend unavailable"));
    backend.log({ text: "unreachable" });
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("range: backend unavailable"),
    );
    expect(screen.queryByText("unreachable")).not.toBeInTheDocument();
  });

  it("is disabled until an app is attached", async () => {
    render(<App backend={createMockBackend()} />);
    expect(await screen.findByRole("textbox", { name: /evaluate expression/i })).toBeDisabled();
  });

  it("expands objects lazily and reports stale handles after a reload", async () => {
    const user = userEvent.setup();
    const backend = attachedBackend();
    backend.setProperties("obj-1", [
      { name: "n", summary: "1", isOwn: true, enumerable: true, writable: true, hasGetter: false },
    ]);
    const objectArgs = [
      { type: "string", value: "tick" },
      { type: "object", className: "Object", description: "Object", objectId: "obj-1" },
    ];
    backend.log(
      { text: "tick Object", args: objectArgs },
      { text: "tick Object", args: objectArgs },
    );
    useConsoleStore.getState().setPreserveLog(true);
    render(<App backend={backend} />);
    const log = await screen.findByRole("log");

    const [first, second] = await within(log).findAllByRole("button", { name: /expand Object/i });
    await user.click(first);
    expect(await within(log).findByText("n:")).toBeInTheDocument();
    expect(within(log).getByText("1")).toBeInTheDocument();
    await waitFor(() => expect(backend.calls).toContain("console.properties:obj-1"));

    backend.resetContext();
    await within(log).findByText(/context was reset/);

    await user.click(second);
    expect(await within(log).findByRole("note")).toHaveTextContent(/no longer available/);
  });

  it("drops entries from before a reload unless preserve log is on", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.log({ text: "before reload" });
    render(<App backend={backend} />);
    await screen.findByText("before reload");

    backend.resetContext();
    await screen.findByText(/context was reset/);
    expect(screen.queryByText("before reload")).not.toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: /preserve log/i }));
    backend.log({ text: "after reload" });
    await screen.findByText("after reload");
    backend.resetContext();
    await waitFor(() => expect(screen.getAllByText(/context was reset/)).toHaveLength(2));
    expect(screen.getByText("after reload")).toBeInTheDocument();
  });
});
