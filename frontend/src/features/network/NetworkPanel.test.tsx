import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, type MockBackend } from "@/backend";
import { makeJwt } from "@/test/jwt";

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
    scrollToIndex: () => {},
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
  return screen.findByRole("grid", { name: "Requests" });
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

  it("decodes a JWT in a header on demand, and only there", async () => {
    const user = userEvent.setup();
    const token = makeJwt({ alg: "RS256", kid: "k1" }, { sub: "ada", exp: 1 });
    const backend = createMockBackend();
    backend.addRequest({
      url: "https://api.example.com/me",
      state: "complete",
      status: 200,
      startTime: 1,
      endTime: 1.1,
      requestHeaders: { Authorization: `Bearer ${token}`, Accept: "application/json" },
      responseHeaders: { Server: "a.b.c" },
    });
    const table = await openPanel(backend);
    // The one request row; the header row says "Method", not "GET".
    await user.click(await within(table).findByRole("row", { name: /GET/ }));
    const detail = await screen.findByRole("region", { name: "Request details" });

    const toggle = await within(detail).findByRole("button", {
      name: "Decode JWT in Authorization",
    });
    expect(within(detail).getAllByRole("button", { name: /^Decode JWT in/ })).toHaveLength(1);
    expect(within(detail).queryByRole("tree", { name: "JWT payload" })).not.toBeInTheDocument();

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(within(detail).getByTestId("jwt-status")).toHaveTextContent(
      /^Expired · expired .* ago$/,
    );
    expect(within(detail).getByRole("tree", { name: "JWT header" })).toHaveTextContent('"k1"');
    expect(within(detail).getByRole("tree", { name: "JWT payload" })).toHaveTextContent('"ada"');
    expect(within(detail).getByText(/Signature not verified/)).toBeInTheDocument();
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

  it("says why the details of a request cannot load", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    vi.spyOn(backend.network, "request").mockRejectedValue(new Error("network: request not found"));
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /json/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("network: request not found");
  });

  it("moves through the requests with the arrow keys from a single tab stop", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    expect(table).toHaveAttribute("aria-rowcount", "3");
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.filter((r) => r.tabIndex === 0)).toHaveLength(1);

    // Newest first by default: logo.png, then json.
    rows[0].focus();
    await user.keyboard("{ArrowDown}");
    const second = within(table).getByRole("row", { name: /json/ });
    expect(second).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(second).toHaveFocus());
    await user.keyboard("{ArrowUp}");
    expect(within(table).getByRole("row", { name: /logo\.png/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("switches detail tabs with the arrow keys", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await user.click(await within(table).findByRole("row", { name: /json/ }));
    const headers = await screen.findByRole("tab", { name: "Headers" });
    expect(headers).toHaveAttribute("aria-controls");
    headers.focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Payload" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Payload" })).toHaveFocus();
  });

  it("includes the request body in Copy as cURL even before the Payload tab loads it", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.addRequest({
      url: "https://api.example.com/todos",
      method: "POST",
      hasPostData: true,
      postData: "a=1",
      state: "complete",
      status: 201,
      endTime: 2,
    });
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /todos/ }));
    await user.click(await screen.findByRole("button", { name: "Copy as cURL" }));
    await waitFor(() => expect(backend.clipboardText).toContain("--data-raw 'a=1'"));
  });

  it("lists the newest request first and sorts by any column", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    const names = () =>
      within(table)
        .getAllByRole("row")
        .slice(1)
        .map((r) => r.textContent ?? "");
    expect(names()[0]).toMatch(/logo\.png/);
    expect(within(table).getByRole("columnheader", { name: /Waterfall/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );

    await user.click(within(table).getByRole("button", { name: "Sort by Status" }));
    expect(within(table).getByRole("columnheader", { name: /Status/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(names()[0]).toMatch(/json/); // 200 sorts before (failed)
    await user.click(within(table).getByRole("button", { name: "Sort by Status" }));
    expect(within(table).getByRole("columnheader", { name: /Status/ })).toHaveAttribute(
      "aria-sort",
      "descending",
    );
    expect(names()[0]).toMatch(/logo\.png/);
  });

  it("keeps only the essential columns while the details are open", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    expect(within(table).getAllByRole("columnheader")).toHaveLength(7);
    await user.click(await within(table).findByRole("row", { name: /json/ }));
    await screen.findByRole("region", { name: "Request details" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Name", "Status", "Time"]);
    expect(screen.getByRole("button", { name: "Copy as cURL" })).toHaveTextContent("Copy as cURL");
    // The Waterfall header that shows the default sort is hidden; the footer says it.
    expect(screen.getByText(/Newest first/)).toBeInTheDocument();
  });

  it("copies the response body from any tab", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /json/ }));

    const copy = await screen.findByRole("button", { name: "Copy response" });
    expect(copy).toHaveTextContent("Copy response");
    await user.click(copy);
    await waitFor(() => expect(backend.clipboardText).toBe('{"title":"Sample"}'));
  });

  it("offers no response copy for a request that failed", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await user.click(await within(table).findByRole("row", { name: /logo\.png/ }));
    await screen.findByRole("button", { name: "Copy as cURL" });
    expect(screen.queryByRole("button", { name: "Copy response" })).not.toBeInTheDocument();
  });

  it("says when Copy as cURL could not include the request body", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.addRequest({
      url: "https://x/upload",
      method: "POST",
      hasPostData: true,
      state: "complete",
      status: 201,
      endTime: 2,
    });
    vi.spyOn(backend.network, "requestBody").mockRejectedValue(
      new Error("network: the request body is larger than Lens shows"),
    );
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /upload/ }));
    await user.click(await screen.findByRole("button", { name: "Copy as cURL" }));
    expect(await screen.findByText(/without the request body/)).toBeInTheDocument();
    expect(backend.clipboardText).toContain("curl 'https://x/upload'");
  });

  it("reports a failed copy when the response body is larger than Lens shows", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    const big = backend.addRequest({
      url: "https://x/big",
      state: "complete",
      status: 200,
      endTime: 2,
    });
    backend.setResponseBody(big.seq, { text: "", base64: false, truncated: true, size: 9_000_000 });
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /big/ }));
    await user.click(await screen.findByRole("button", { name: "Copy response" }));
    expect(await screen.findByRole("button", { name: "Copy response" })).toHaveTextContent(
      "Copy failed",
    );
  });

  it("reuses the body Copy response fetched when the Response tab opens", async () => {
    const user = userEvent.setup();
    const backend = withRequests();
    const table = await openPanel(backend);
    const row = await within(table).findByRole("row", { name: /json/ });
    await user.click(row);
    await user.click(await screen.findByRole("button", { name: "Copy response" }));
    await waitFor(() => expect(backend.clipboardText).toBe('{"title":"Sample"}'));
    await user.click(screen.getByRole("tab", { name: "Response" }));
    await screen.findByText('{"title":"Sample"}');
    expect(backend.calls.filter((c) => c.startsWith("network.responseBody:"))).toHaveLength(1);
  });

  it("shows the query string, request body, timing and initiator", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.addRequest({
      url: "https://api.example.com/todos?page=2&q=a%20b",
      method: "POST",
      hasPostData: true,
      postData: '{"title":"x"}',
      state: "complete",
      status: 201,
      startTime: 10,
      responseTime: 10.2,
      endTime: 10.3,
      timing: {
        requestTime: 10,
        proxyStart: -1,
        proxyEnd: -1,
        dnsStart: 1,
        dnsEnd: 4,
        connectStart: 4,
        connectEnd: 20,
        sslStart: -1,
        sslEnd: -1,
        sendStart: 21,
        sendEnd: 22,
        receiveHeadersEnd: 200,
      },
      initiator: { type: "script", url: "index.bundle", lineNumber: 41 },
    });
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /todos/ }));
    const detail = await screen.findByRole("region", { name: "Request details" });

    await user.click(within(detail).getByRole("tab", { name: "Payload" }));
    expect(await within(detail).findByText("page")).toBeInTheDocument();
    expect(within(detail).getByText("a b")).toBeInTheDocument();
    expect(await within(detail).findByText('"x"')).toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "Timing" }));
    expect(within(detail).getByText("DNS lookup")).toBeInTheDocument();
    expect(within(detail).getByText("Waiting for server")).toBeInTheDocument();
    expect(within(detail).queryByText("SSL")).not.toBeInTheDocument();
    expect(within(detail).getByText("Total 300 ms")).toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "Initiator" }));
    expect(within(detail).getByText("script")).toBeInTheDocument();
    expect(within(detail).getByText("index.bundle:42")).toBeInTheDocument();
  });

  it("shows the error in the Response tab when Copy response could not fetch the body", async () => {
    const user = userEvent.setup();
    const backend = createMockBackend();
    backend.addRequest({ url: "https://x/gone", state: "complete", status: 200, endTime: 2 }); // no body set: fetch fails
    const table = await openPanel(backend);
    await user.click(await within(table).findByRole("row", { name: /gone/ }));
    await user.click(await screen.findByRole("button", { name: "Copy response" }));
    expect(await screen.findByRole("button", { name: "Copy response" })).toHaveTextContent(
      "Copy failed",
    );
    await user.click(screen.getByRole("tab", { name: "Response" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("network: no body");
  });

  it("jumps to the first and last request with Home and End", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    within(table).getAllByRole("row")[1].focus();
    await user.keyboard("{End}");
    const last = within(table).getByRole("row", { name: /json/ });
    expect(last).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(last).toHaveFocus());
    await user.keyboard("{Home}");
    expect(within(table).getByRole("row", { name: /logo\.png/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("keeps an earlier notice when Copy as cURL includes everything", async () => {
    const user = userEvent.setup();
    const table = await openPanel(withRequests());
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    await user.click(screen.getByRole("button", { name: "Export HAR" }));
    expect(await screen.findByText("Saved /Users/me/lens.har")).toBeInTheDocument();
    await user.click(within(table).getByRole("row", { name: /json/ }));
    await user.click(await screen.findByRole("button", { name: "Copy as cURL" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Copy as cURL" })).toHaveTextContent("Copied"),
    );
    expect(screen.getByText("Saved /Users/me/lens.har")).toBeInTheDocument();
  });
});
