import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, sampleDevice, sampleQuery } from "@/backend";

import { useQueriesStore } from "./queriesStore";

describe("QueriesPanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "queries" });
    useQueriesStore.getState().reset();
  });

  it("explains how to install the SDK when no app is connected", async () => {
    render(<App backend={createMockBackend()} />);
    expect(
      await screen.findByText(/No app has connected through the Lens SDK/),
    ).toBeInTheDocument();
    expect(screen.getByText(/ws:\/\/127\.0\.0\.1:7420\/lens/)).toBeInTheDocument();
  });

  it("lists queries with their status once a device connects and filters them", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    render(<App backend={backend} />);
    await screen.findByText(/No app has connected/);

    backend.connectDevice(sampleDevice(), [
      sampleQuery(),
      sampleQuery({ queryHash: '["user"]', queryKey: ["user"], label: "stale", isStale: true }),
    ]);

    const list = await screen.findByRole("listbox", { name: "Queries" });
    expect(within(list).getAllByRole("option")).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Stale (1)" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Stale (1)" }));
    expect(within(list).getAllByRole("option")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "Stale (1)" }));
    await user.type(screen.getByRole("searchbox", { name: /filter queries/i }), "user");
    expect(within(list).getAllByRole("option")).toHaveLength(1);
    expect(within(list).getByText('["user"]')).toBeInTheDocument();
  });

  it("pulls the chosen device's queries when the picker changes", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    render(<App backend={backend} />);
    await screen.findByText(/No app has connected/);

    backend.connectDevice(sampleDevice(), [sampleQuery()]);
    backend.connectDevice(
      sampleDevice({ key: "com.other|device-2", device: { id: "device-2", name: "Pixel 9" } }),
      [sampleQuery({ queryHash: '["user"]', queryKey: ["user"] })],
    );
    const list = await screen.findByRole("listbox", { name: "Queries" });
    expect(within(list).getByText('["todos",{"page":1}]')).toBeInTheDocument();

    // Switching device must pull that device's cache without waiting for a notice.
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Connected app" }),
      "com.other|device-2",
    );
    expect(await within(list).findByText('["user"]')).toBeInTheDocument();
    expect(within(list).queryByText('["todos",{"page":1}]')).not.toBeInTheDocument();
  });

  it("runs actions on the selected query and edits its data", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.connectDevice(sampleDevice(), [sampleQuery()]);
    render(<App backend={backend} />);

    await user.click(await screen.findByRole("option"));
    await user.click(screen.getByRole("button", { name: "Invalidate" }));
    await waitFor(() =>
      expect(backend.calls).toContain('queries.action:INVALIDATE:["todos",{"page":1}]'),
    );
    expect(await screen.findByText("invalidated")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Edit data" }));
    const editor = screen.getByRole("textbox", { name: /query data json/i });
    await user.clear(editor);
    await user.type(editor, "[[{{]");
    await user.click(screen.getByRole("button", { name: "Save to app" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();

    await user.clear(editor);
    await user.type(editor, '{{"edited": true}');
    await user.click(screen.getByRole("button", { name: "Save to app" }));
    await waitFor(() =>
      expect(backend.calls.some((c) => c.startsWith("queries.action:SET_QUERY_DATA"))).toBe(true),
    );
    expect(await screen.findByText("edited:")).toBeInTheDocument();
  });

  it("toggles the device online state", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.connectDevice(sampleDevice(), [sampleQuery()]);
    render(<App backend={backend} />);

    await user.click(await screen.findByRole("button", { name: /simulate offline/i }));
    await waitFor(() => expect(backend.calls).toContain("queries.setOnline:false"));
    expect(await screen.findByRole("button", { name: /go back online/i })).toBeInTheDocument();
  });

  it("shows mutations with variables and data", async () => {
    const user = userEvent.setup();
    useShellStore.setState({ activePanel: "mutations" });
    const backend = createMockBackend();
    backend.connectDevice(
      sampleDevice(),
      [],
      [
        {
          mutationId: 7,
          mutationKey: ["addTodo"],
          state: {
            status: "success",
            submittedAt: Date.now(),
            isPaused: false,
            variables: { title: "x" },
            data: { id: 9 },
          },
        },
      ],
    );
    render(<App backend={backend} />);

    await user.click(await screen.findByRole("option", { name: /addTodo/ }));
    expect(screen.getByText("title:")).toBeInTheDocument();
    expect(screen.getByText('"x"')).toBeInTheDocument();
  });
});
