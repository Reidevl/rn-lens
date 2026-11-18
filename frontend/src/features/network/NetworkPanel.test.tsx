import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, type MockBackend } from "@/backend";

import { useNetworkStore } from "./networkStore";

// jsdom has no layout; render every row.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 24,
        size: 24,
      })),
    getTotalSize: () => count * 24,
    measureElement: () => {},
  }),
}));

function withRequests(): MockBackend {
  const backend = createMockBackend();
  const json = backend.addRequest({
    url: "https://httpbin.org/json",
    state: "complete",
    status: 200,
    statusText: "OK",
    mimeType: "application/json",
    startTime: 10,
    responseTime: 10.1,
    endTime: 10.25,
    encodedSize: 429,
    requestHeaders: { Accept: "application/json" },
    responseHeaders: { "Content-Type": "application/json" },
  });
  backend.setResponseBody(json.seq, '{"title":"Sample"}');
  backend.addRequest({
    url: "https://cdn.example.com/logo.png",
    resourceType: "Image",
    state: "failed",
    errorText: "net::ERR_FAILED",
    startTime: 10.3,
    endTime: 10.4,
  });
  return backend;
}

async function openPanel(backend: MockBackend) {
  render(<App backend={backend} />);
  return screen.findByRole("table", { name: "Requests" });
}

describe("NetworkPanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "network" });
    useNetworkStore.getState().reset();
  });

  it("explains when the app cannot report network traffic", async () => {
    const backend = createMockBackend();
    backend.setNetworkAvailability({
      checked: true,
      supported: false,
      reason: "Network inspection needs RN 0.83+",
    });
    render(<App backend={backend} />);
    expect(await screen.findByText("Network inspection needs RN 0.83+")).toBeInTheDocument();
  });

  it("asks to attach before the first check", async () => {
    const backend = createMockBackend();
    backend.setNetworkAvailability({ checked: false, supported: false });
    render(<App backend={backend} />);
    expect(await screen.findByText(/Attach to an app/)).toBeInTheDocument();
  });

  it("lists requests and filters them by type and URL", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    const json = within(table).getByRole("row", { name: /json/ });
    expect(json).toHaveTextContent("GET");
    expect(json).toHaveTextContent("200");
    expect(json).toHaveTextContent("429 B");
    expect(json).toHaveTextContent("250 ms");
    expect(within(table).getByRole("row", { name: /logo\.png/ })).toHaveTextContent("(failed)");

    await user.click(screen.getByRole("button", { name: "Img" }));
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Img" }));

    await user.type(screen.getByRole("searchbox", { name: "Filter requests by URL" }), "httpbin");
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    expect(within(table).getByRole("row", { name: /json/ })).toBeInTheDocument();
  });

  it("shows headers, the response and a JSON preview for the selected request", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /json/ }));

    const detail = await screen.findByRole("region", { name: "Request details" });
    expect(await within(detail).findByText("Accept")).toBeInTheDocument();
    expect(within(detail).getByText("https://httpbin.org/json")).toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "Response" }));
    expect(await within(detail).findByText('{"title":"Sample"}')).toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "Preview" }));
    expect(await within(detail).findByText('"Sample"')).toBeInTheDocument();
  });

  it("copies the selected request as cURL", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /json/ }));

    await user.click(await screen.findByRole("button", { name: "Copy as cURL" }));
    await waitFor(() =>
      expect(backend.clipboardText).toBe(
        "curl 'https://httpbin.org/json' \\\n  -H 'Accept: application/json'",
      ),
    );
  });

  it("exports a HAR and clears the log", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    const table = await openPanel(backend);
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));

    await user.click(screen.getByRole("button", { name: "Export HAR" }));
    expect(await screen.findByText("Saved /Users/me/lens.har")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Clear" }));
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(1));
    expect(backend.calls).toContain("network.clear");
  });
});
