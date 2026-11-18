import { useVirtualizer } from "@tanstack/react-virtual";
import { Download, Plus, RefreshCw, Search, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";

import {
  useBackend,
  type StorageKeyInfo,
  type StorageStore,
  type StorageValueType,
} from "@/backend";
import { cn } from "@/shared/lib/cn";
import { formatBytes } from "@/shared/lib/formatBytes";
import { Button } from "@/shared/ui/atoms/Button";

import { KIND_LABEL } from "./presentation";
import { loadValue, runStorageAction, useStorageStore } from "./storageStore";
import { useStorageSync } from "./useStorageSync";
import { ValueView } from "./ValueView";

const VIRTUALIZE_ABOVE = 300;

const storeLabel = (s: StorageStore) =>
  s.name && s.name !== KIND_LABEL[s.kind]
    ? `${KIND_LABEL[s.kind]} · ${s.name}`
    : KIND_LABEL[s.kind];

export function StoragePanel() {
  const backend = useBackend();
  useStorageSync();
  const devices = useStorageStore((s) => s.devices);
  const deviceKey = useStorageStore((s) => s.deviceKey);
  const stores = useStorageStore((s) => s.stores);
  const storeId = useStorageStore((s) => s.storeId);
  const selectedKey = useStorageStore((s) => s.selectedKey);
  const filter = useStorageStore((s) => s.filter);
  const busy = useStorageStore((s) => s.busy);
  const error = useStorageStore((s) => s.error);
  const notice = useStorageStore((s) => s.notice);
  const { selectDevice, selectStore, selectKey, setFilter } = useStorageStore.getState();
  const [adding, setAdding] = useState(false);
  // Armed for one device and store only: switching disarms it.
  const [clearArmedFor, setClearArmedFor] = useState<string | null>(null);

  const store = stores.find((s) => s.id === storeId) ?? null;
  const clearTarget = deviceKey && store ? `${deviceKey}\n${store.id}` : null;
  const confirmClear = clearArmedFor !== null && clearArmedFor === clearTarget;
  const keys = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    const all = store?.keys ?? [];
    return needle ? all.filter((k) => k.key.toLowerCase().includes(needle)) : all;
  }, [store, filter]);

  useEffect(() => {
    if (selectedKey) void loadValue(backend);
  }, [backend, deviceKey, storeId, selectedKey]);

  const act = (fn: () => Promise<string | void>) => void runStorageAction(fn);

  if (!deviceKey) {
    return (
      <Shell>
        <NoStorage />
      </Shell>
    );
  }

  return (
    <Shell
      actions={
        store ? (
          <>
            <Button size="sm" disabled={busy} onClick={() => setAdding((v) => !v)}>
              <Plus aria-hidden="true" className="size-3.5" />
              Add key
            </Button>
            <Button
              size="sm"
              disabled={busy}
              onClick={() => act(() => backend.storage.refresh(deviceKey, store.id))}
            >
              <RefreshCw aria-hidden="true" className="size-3.5" />
              Refresh
            </Button>
            <Button
              size="sm"
              disabled={busy}
              onClick={() =>
                act(async () => {
                  const path = await backend.storage.exportStore(deviceKey, store.id);
                  return path ? `Exported to ${path}` : undefined;
                })
              }
            >
              <Download aria-hidden="true" className="size-3.5" />
              Export
            </Button>
            <Button
              size="sm"
              disabled={busy}
              onClick={() =>
                act(async () => {
                  const res = await backend.storage.importStore(deviceKey, store.id);
                  if (!res.path) return undefined;
                  const skipped = Object.entries(res.skipped ?? {});
                  const detail = skipped.length
                    ? `; skipped ${skipped.length} (${skipped.map(([k, why]) => `${k}: ${why}`).join("; ")})`
                    : "";
                  return `Imported ${res.written} keys from ${res.path}${detail}`;
                })
              }
            >
              <Upload aria-hidden="true" className="size-3.5" />
              Import
            </Button>
            {confirmClear ? (
              <Button
                size="sm"
                variant="destructive"
                disabled={busy}
                onClick={() => {
                  setClearArmedFor(null);
                  act(() => backend.storage.clear(deviceKey, store.id));
                }}
              >
                Confirm clear
              </Button>
            ) : (
              <Button size="sm" disabled={busy} onClick={() => setClearArmedFor(clearTarget)}>
                <Trash2 aria-hidden="true" className="size-3.5" />
                Clear store
              </Button>
            )}
          </>
        ) : null
      }
      onKeyDown={(e) => {
        // Escape disarms a pending confirmation.
        if (e.key === "Escape") setClearArmedFor(null);
      }}
      device={
        devices.length > 1 ? (
          <select
            aria-label="Device"
            value={deviceKey}
            onChange={(e) => {
              setClearArmedFor(null);
              selectDevice(e.target.value);
            }}
            className="h-7 rounded-md border border-border bg-background px-1 text-xs"
          >
            {devices.map((d) => (
              <option key={d.key} value={d.key}>
                {d.app.name ?? d.app.id} · {d.device.name ?? d.device.id}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-xs text-muted-foreground">
            {devices[0]?.app.name ?? devices[0]?.app.id} · {devices[0]?.app.platform}
          </span>
        )
      }
    >
      {error ? (
        <p
          role="alert"
          className="border-b border-destructive/60 bg-destructive/10 px-3 py-1 text-xs"
        >
          {error}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="border-b border-border bg-muted/40 px-3 py-1 text-xs">
          {notice}
        </p>
      ) : null}

      {stores.length === 0 ? (
        <div className="flex flex-1 items-center justify-center p-6 text-sm text-muted-foreground">
          The app connected but exposes no stores.
        </div>
      ) : (
        <>
          <div
            role="tablist"
            aria-label="Stores"
            className="flex flex-wrap gap-1 border-b border-border px-3 py-1.5"
            onKeyDown={(e) => {
              const i = stores.findIndex((s) => s.id === storeId);
              const next =
                e.key === "ArrowRight"
                  ? (i + 1) % stores.length
                  : e.key === "ArrowLeft"
                    ? (i - 1 + stores.length) % stores.length
                    : -1;
              if (next < 0) return;
              e.preventDefault();
              setAdding(false);
              setClearArmedFor(null);
              selectStore(stores[next].id);
              document.getElementById(`storage-store-${stores[next].id}`)?.focus();
            }}
          >
            {stores.map((s) => (
              <button
                key={s.id}
                id={`storage-store-${s.id}`}
                type="button"
                role="tab"
                aria-selected={s.id === storeId}
                aria-controls="storage-store-panel"
                tabIndex={s.id === storeId ? 0 : -1}
                onClick={() => {
                  setAdding(false);
                  setClearArmedFor(null);
                  selectStore(s.id);
                }}
                className={cn(
                  "flex h-6 cursor-pointer items-center gap-1.5 rounded-md px-2 text-xs transition-colors duration-150",
                  s.id === storeId
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:bg-muted/60",
                )}
              >
                {storeLabel(s)}
                <span className="tabular-nums opacity-70">{s.keys?.length ?? 0}</span>
              </button>
            ))}
          </div>
          {store?.live ? (
            <p className="border-b border-border px-3 py-1 text-xs text-muted-foreground">
              Live: changes appear as the app makes them.
            </p>
          ) : store?.declaredKeys ? (
            <p className="border-b border-border px-3 py-1 text-xs text-muted-foreground">
              SecureStore cannot list its keys: Lens only lists the keys the app declared in{" "}
              <code className="font-mono">storage.secureStore.keys</code>.
            </p>
          ) : null}
          {store?.truncated ? (
            <p className="border-b border-border px-3 py-1 text-xs text-warning">
              This store has more keys than the app sends (10,000).
            </p>
          ) : null}
          {adding && store ? (
            <AddKey
              onCancel={() => setAdding(false)}
              onAdd={async (key, type, value) => {
                const ok = await runStorageAction(() =>
                  backend.storage.set(deviceKey, store.id, key, type, value),
                );
                if (ok) {
                  setAdding(false);
                  selectKey(key);
                }
              }}
            />
          ) : null}
          <div
            id="storage-store-panel"
            role="tabpanel"
            aria-labelledby={store ? `storage-store-${store.id}` : undefined}
            className={cn(
              "grid min-h-0 flex-1",
              selectedKey ? "grid-cols-[minmax(14rem,2fr)_minmax(20rem,3fr)]" : "grid-cols-1",
            )}
          >
            <div className="flex min-h-0 min-w-0 flex-col">
              <label className="relative flex items-center border-b border-border px-2 py-1.5">
                <Search
                  aria-hidden="true"
                  className="pointer-events-none absolute left-4 size-3.5 text-muted-foreground"
                />
                <input
                  type="search"
                  aria-label="Filter keys"
                  placeholder="Filter keys"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  className="h-7 w-full rounded-md border border-border bg-background pr-2 pl-7 text-sm outline-none focus-visible:outline-2 focus-visible:outline-ring"
                />
              </label>
              <KeyList
                keys={keys}
                selected={selectedKey}
                onSelect={selectKey}
                empty={(store?.keys?.length ?? 0) === 0}
              />
            </div>
            {selectedKey && store ? (
              <ValueView
                key={`${store.id}/${selectedKey}`}
                deviceKey={deviceKey}
                storeId={store.id}
                valueKey={selectedKey}
              />
            ) : null}
          </div>
        </>
      )}
    </Shell>
  );
}

function Shell({
  children,
  actions,
  device,
  onKeyDown,
}: {
  children: React.ReactNode;
  actions?: React.ReactNode;
  device?: React.ReactNode;
  onKeyDown?: (e: React.KeyboardEvent) => void;
}) {
  return (
    <section
      aria-labelledby="panel-storage-title"
      className="flex h-full flex-col"
      onKeyDown={onKeyDown}
    >
      <header className="flex h-12 items-center gap-2 border-b border-border px-3">
        <h1 id="panel-storage-title" className="text-sm font-semibold">
          Storage
        </h1>
        {device}
        <div className="flex-1" />
        {actions}
      </header>
      {children}
    </section>
  );
}

function KeyRow({
  k,
  selected,
  onSelect,
}: {
  k: StorageKeyInfo;
  selected: boolean;
  onSelect: (key: string) => void;
}) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      onClick={() => onSelect(k.key)}
      className={cn(
        "flex h-7 w-full cursor-pointer items-center gap-2 border-b border-border/40 px-3 text-left text-sm",
        selected ? "bg-muted" : "hover:bg-muted/50",
      )}
    >
      <span className="min-w-0 flex-1 truncate font-mono">{k.key}</span>
      {k.size != null ? (
        <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
          {formatBytes(k.size)}
        </span>
      ) : null}
    </button>
  );
}

function KeyList({
  keys,
  selected,
  onSelect,
  empty,
}: {
  keys: StorageKeyInfo[];
  selected: string | null;
  onSelect: (key: string) => void;
  empty: boolean;
}) {
  if (keys.length === 0) {
    return (
      <p className="p-4 text-sm text-muted-foreground">
        {empty ? "This store is empty." : "No keys match the filter."}
      </p>
    );
  }
  if (keys.length > VIRTUALIZE_ABOVE)
    return <VirtualKeyList keys={keys} selected={selected} onSelect={onSelect} />;
  return (
    <div role="listbox" aria-label="Keys" className="min-h-0 flex-1 overflow-auto">
      {keys.map((k) => (
        <KeyRow key={k.key} k={k} selected={k.key === selected} onSelect={onSelect} />
      ))}
    </div>
  );
}

function VirtualKeyList({
  keys,
  selected,
  onSelect,
}: {
  keys: StorageKeyInfo[];
  selected: string | null;
  onSelect: (key: string) => void;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  // TanStack Virtual keeps its own mutable instance, which the React Compiler
  // cannot memoize; the component is small enough to skip compilation.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: keys.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 28,
    overscan: 20,
    getItemKey: (i) => keys[i].key,
  });
  return (
    <div ref={parentRef} role="listbox" aria-label="Keys" className="min-h-0 flex-1 overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((item) => (
          <div
            key={item.key}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              transform: `translateY(${item.start}px)`,
            }}
          >
            <KeyRow
              k={keys[item.index]}
              selected={keys[item.index].key === selected}
              onSelect={onSelect}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

function AddKey({
  onAdd,
  onCancel,
}: {
  onAdd: (key: string, type: StorageValueType, value: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [key, setKey] = useState("");
  const [type, setType] = useState<StorageValueType>("string");
  const [value, setValue] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (key.trim()) void onAdd(key.trim(), type, value);
  };
  return (
    <form
      aria-label="Add key"
      onSubmit={submit}
      className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2 text-xs"
    >
      <input
        aria-label="Key"
        // The form opens on request, so its first field takes focus.
        autoFocus
        placeholder="key"
        value={key}
        onChange={(e) => setKey(e.target.value)}
        className="h-7 w-40 rounded-md border border-border bg-background px-2 font-mono"
      />
      <select
        aria-label="Type"
        value={type}
        onChange={(e) => setType(e.target.value as StorageValueType)}
        className="h-7 rounded-md border border-border bg-background px-1"
      >
        {(["string", "number", "boolean", "bytes"] as const).map((t) => (
          <option key={t}>{t}</option>
        ))}
      </select>
      <input
        aria-label="Value"
        placeholder={type === "bytes" ? "base64" : "value"}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="h-7 min-w-40 flex-1 rounded-md border border-border bg-background px-2 font-mono"
      />
      <Button size="sm" variant="primary" type="submit" disabled={!key.trim()}>
        Add
      </Button>
      <Button size="sm" onClick={onCancel}>
        Cancel
      </Button>
    </form>
  );
}

const SNIPPET = `import AsyncStorage from "@react-native-async-storage/async-storage";
import { createMMKV } from "react-native-mmkv";
import * as SecureStore from "expo-secure-store";

useLens({ storage: {
  asyncStorage: AsyncStorage,
  mmkv: { default: createMMKV() },
  secureStore: { module: SecureStore, keys: ["token"] },
} });`;

function NoStorage() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-sm text-muted-foreground">
      <p className="font-medium text-foreground">No app exposes its storage to Lens yet.</p>
      <p>Pass the stores you use to the hook (each is optional):</p>
      <pre className="rounded-md border border-border bg-card p-3 font-mono text-xs text-foreground">
        {SNIPPET}
      </pre>
    </div>
  );
}
