import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "@/app/App";
import { useShellStore } from "@/app/shell/shellStore";
import { createMockBackend, type MockBackend, type StorageDevice } from "@/backend";

import { useStorageStore } from "./storageStore";

const device: StorageDevice = {
  key: "dev.rnlens.example|d1",
  sessionId: "s1",
  app: { id: "dev.rnlens.example", name: "Lens Example", platform: "ios" },
  device: { id: "d1", name: "iPhone" },
  sdkVersion: "0.1.0",
  connectedAt: "2026-01-01T00:00:00Z",
};

function withStorage(): MockBackend {
  const backend = createMockBackend();
  backend.connectStorage(
    device,
    [
      {
        id: "async-storage",
        kind: "async-storage",
        name: "AsyncStorage",
        keys: [
          { key: "profile", size: 30 },
          { key: "theme", size: 4 },
        ],
      },
      {
        id: "mmkv:default",
        kind: "mmkv",
        name: "default",
        live: true,
        keys: [
          { key: "launches", size: 2 },
          { key: "avatar", size: 3 },
        ],
      },
      {
        id: "secure-store",
        kind: "secure-store",
        name: "SecureStore",
        declaredKeys: true,
        keys: [{ key: "token" }],
      },
    ],
    {
      "async-storage/profile": { type: "string", value: '{"name":"Ada","plan":"pro"}' },
      "async-storage/theme": { type: "string", value: "dark" },
      "mmkv:default/launches": { type: "number", value: "12" },
      "mmkv:default/avatar": { type: "bytes", value: "AAH/" },
      "secure-store/token": { type: "string", value: "", truncated: true, size: 2_000_000 },
    },
  );
  return backend;
}

async function openKey(user: ReturnType<typeof userEvent.setup>, key: string) {
  await user.click(await screen.findByRole("option", { name: new RegExp(`^${key}`) }));
  return screen.findByRole("region", { name: "Value" });
}

describe("StoragePanel", () => {
  beforeEach(() => {
    useShellStore.setState({ activePanel: "storage" });
    useStorageStore.getState().reset();
  });

  it("explains how to expose storage when no app does", async () => {
    render(<App backend={createMockBackend()} />);
    expect(await screen.findByText(/No app exposes its storage to Lens yet/)).toBeInTheDocument();
    expect(screen.getByText(/useLens\(\{ storage:/)).toBeInTheDocument();
  });

  it("lists the stores and filters their keys", async () => {
    const user = userEvent.setup();
    render(<App backend={withStorage()} />);
    const tabs = await screen.findByRole("tablist", { name: "Stores" });
    expect(
      within(tabs)
        .getAllByRole("tab")
        .map((t) => t.textContent),
    ).toEqual([
      expect.stringContaining("AsyncStorage"),
      expect.stringContaining("MMKV · default"),
      expect.stringContaining("SecureStore"),
    ]);
    const keys = screen.getByRole("listbox", { name: "Keys" });
    expect(within(keys).getAllByRole("option")).toHaveLength(2);
    await user.type(screen.getByRole("searchbox", { name: "Filter keys" }), "the");
    expect(
      within(keys)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual([expect.stringContaining("theme")]);

    await user.click(within(tabs).getByRole("tab", { name: /MMKV/ }));
    expect(screen.getByText(/Live: changes appear as the app makes them/)).toBeInTheDocument();
    await user.click(within(tabs).getByRole("tab", { name: /SecureStore/ }));
    expect(screen.getByText(/only lists the keys the app declared/)).toBeInTheDocument();
  });

  it("shows a value as text, JSON, hex and base64", async () => {
    const user = userEvent.setup();
    render(<App backend={withStorage()} />);
    const value = await openKey(user, "profile");
    expect(await within(value).findByText('{"name":"Ada","plan":"pro"}')).toBeInTheDocument();

    await user.click(within(value).getByRole("tab", { name: "JSON" }));
    expect(within(value).getByText('"Ada"')).toBeInTheDocument();
    await user.click(within(value).getByRole("tab", { name: "Hex" }));
    expect(within(value).getByText(/^00000000 7b 22 6e 61/)).toBeInTheDocument();
    await user.click(within(value).getByRole("tab", { name: "Base64" }));
    expect(within(value).getByText(/^eyJuYW1lIjoiQWRhIiwicGxhbiI6InBybyJ9$/)).toBeInTheDocument();
  });

  it("edits a value and writes it to the app", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    const value = await openKey(user, "theme");
    await within(value).findByText("dark");
    await user.click(within(value).getByRole("button", { name: "Edit" }));
    const editor = within(value).getByRole("textbox", { name: "Value" });
    await user.clear(editor);
    await user.type(editor, "light");
    await user.click(within(value).getByRole("button", { name: "Save to app" }));
    await waitFor(() =>
      expect(backend.storageValue(device.key, "async-storage", "theme")).toEqual({
        type: "string",
        value: "light",
      }),
    );
    expect(await within(value).findByText("light")).toBeInTheDocument();
  });

  it("refuses JSON that does not parse when the value was JSON", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    const value = await openKey(user, "profile");
    await within(value).findByText('{"name":"Ada","plan":"pro"}');
    await user.click(within(value).getByRole("button", { name: "Edit" }));
    const editor = within(value).getByRole("textbox", { name: "Value" });
    await user.clear(editor);
    await user.type(editor, "{{broken");
    await user.click(within(value).getByRole("button", { name: "Save to app" }));
    expect(await within(value).findByRole("alert")).toHaveTextContent(/not valid JSON/);
    expect(backend.calls.some((c) => c.startsWith("storage.set:"))).toBe(false);
  });

  it("adds a key, deletes a key and clears a store after confirmation", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    await screen.findByRole("listbox", { name: "Keys" });

    await user.click(screen.getByRole("button", { name: "Add key" }));
    const form = screen.getByRole("form", { name: "Add key" });
    await user.type(within(form).getByRole("textbox", { name: "Key" }), "onboarded");
    await user.type(within(form).getByRole("textbox", { name: "Value" }), "true");
    await user.click(within(form).getByRole("button", { name: "Add" }));
    await waitFor(() =>
      expect(backend.storageValue(device.key, "async-storage", "onboarded")?.value).toBe("true"),
    );
    expect(await screen.findByRole("option", { name: /^onboarded/ })).toBeInTheDocument();

    const value = await openKey(user, "theme");
    await user.click(within(value).getByRole("button", { name: "Delete" }));
    expect(backend.calls).not.toContain("storage.remove:async-storage/theme");
    await user.click(within(value).getByRole("button", { name: "Confirm delete" }));
    await waitFor(() => expect(backend.calls).toContain("storage.remove:async-storage/theme"));

    await user.click(screen.getByRole("button", { name: "Clear store" }));
    await user.click(screen.getByRole("button", { name: "Confirm clear" }));
    await waitFor(() => expect(backend.calls).toContain("storage.clear:async-storage"));
  });

  it("shows bytes, numbers and values too large to show", async () => {
    const user = userEvent.setup();
    render(<App backend={withStorage()} />);
    await user.click(await screen.findByRole("tab", { name: /MMKV/ }));
    let value = await openKey(user, "avatar");
    expect(await within(value).findByText(/3 B · bytes/)).toBeInTheDocument();
    expect(within(value).getByText(/^00000000 00 01 ff/)).toBeInTheDocument();

    value = await openKey(user, "launches");
    expect(await within(value).findByText("12")).toBeInTheDocument();
    expect(within(value).getByText(/number/)).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /SecureStore/ }));
    value = await openKey(user, "token");
    expect(await within(value).findByText(/larger than Lens shows/)).toBeInTheDocument();
    expect(within(value).queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("exports and imports a store", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    await screen.findByRole("listbox", { name: "Keys" });
    await user.click(screen.getByRole("button", { name: "Export" }));
    expect(
      await screen.findByText("Exported to /Users/me/lens-async-storage.json"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Import" }));
    expect(
      await screen.findByText(/Imported 2 keys from \/Users\/me\/backup\.json; skipped 1/),
    ).toBeInTheDocument();
  });

  it("pulls the chosen device's stores when the device changes", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    backend.connectStorage(
      {
        ...device,
        key: "other|d2",
        sessionId: "s2",
        app: { id: "other", name: "Other App", platform: "android" },
        device: { id: "d2", name: "Pixel" },
      },
      [{ id: "mmkv:cache", kind: "mmkv", name: "cache", live: true, keys: [{ key: "etag" }] }],
    );
    render(<App backend={backend} />);
    await screen.findByRole("listbox", { name: "Keys" });
    await user.selectOptions(screen.getByRole("combobox", { name: "Device" }), "other|d2");
    expect(await screen.findByRole("tab", { name: /MMKV · cache/ })).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: /^etag/ })).toBeInTheDocument();
  });
  it("refreshes the open value when the app changes it", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    await user.click(await screen.findByRole("tab", { name: /MMKV/ }));
    const value = await openKey(user, "launches");
    expect(await within(value).findByText("12")).toBeInTheDocument();

    // A live MMKV write by the app arrives as storage:changed.
    await backend.storage.set(device.key, "mmkv:default", "launches", "number", "13");
    expect(await within(value).findByText("13")).toBeInTheDocument();
  });
  it("disarms Clear store when the device or store changes", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    backend.connectStorage(
      {
        ...device,
        key: "other|d2",
        sessionId: "s2",
        app: { id: "other", name: "Other", platform: "android" },
      },
      [{ id: "async-storage", kind: "async-storage", name: "AsyncStorage", keys: [{ key: "x" }] }],
    );
    render(<App backend={backend} />);
    await screen.findByRole("listbox", { name: "Keys" });

    await user.click(screen.getByRole("button", { name: "Clear store" }));
    await user.click(screen.getByRole("tab", { name: /MMKV/ }));
    expect(screen.queryByRole("button", { name: "Confirm clear" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Clear store" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Device" }), "other|d2");
    await screen.findByRole("option", { name: /^x/ });
    expect(screen.queryByRole("button", { name: "Confirm clear" })).not.toBeInTheDocument();
    expect(backend.calls.some((c) => c.startsWith("storage.clear:"))).toBe(false);
  });

  it("cancels an armed confirmation with Escape and announces results", async () => {
    const user = userEvent.setup();
    const backend = withStorage();
    render(<App backend={backend} />);
    await screen.findByRole("listbox", { name: "Keys" });
    await user.click(screen.getByRole("button", { name: "Clear store" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("button", { name: "Confirm clear" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Export" }));
    expect(
      await screen.findByText("Exported to /Users/me/lens-async-storage.json"),
    ).toHaveAttribute("role", "status");
  });
});
