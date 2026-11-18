import { useState } from "react";

import { useBackend, type StorageValue, type StorageValueType } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { formatBytes } from "@/shared/lib/formatBytes";
import { Button } from "@/shared/ui/atoms/Button";
import { CopyButton } from "@/shared/ui/molecules/CopyButton";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

import { hexDump, jsonOf, toBase64, valueSize } from "./presentation";
import { loadValue, runStorageAction, useStorageStore, valueId } from "./storageStore";

type View = "Text" | "JSON" | "Hex" | "Base64";

/** The opened value: views, copy, edit and delete. */
export function ValueView({
  deviceKey,
  storeId,
  valueKey,
}: {
  deviceKey: string;
  storeId: string;
  valueKey: string;
}) {
  const backend = useBackend();
  const value = useStorageStore((s) => s.value);
  const busy = useStorageStore((s) => s.busy);
  const [view, setView] = useState<View | null>(null);
  const [editing, setEditing] = useState<{ type: StorageValueType; text: string } | null>(null);
  const [draftError, setDraftError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  const loaded =
    value?.id === valueId(deviceKey, storeId, valueKey)
      ? value.loaded
      : { state: "loading" as const };

  const save = async (v: StorageValue) => {
    if (!editing) return;
    if (
      jsonOf(v).ok &&
      editing.type === "string" &&
      !jsonOf({ type: "string", value: editing.text }).ok
    ) {
      setDraftError("The value was JSON and this is not valid JSON.");
      return;
    }
    setDraftError(null);
    const ok = await runStorageAction(() =>
      backend.storage.set(deviceKey, storeId, valueKey, editing.type, editing.text),
    );
    if (ok) {
      setEditing(null);
      await loadValue(backend);
    }
  };

  const remove = async () => {
    const ok = await runStorageAction(() => backend.storage.remove(deviceKey, storeId, valueKey));
    if (ok) useStorageStore.getState().selectKey(null);
  };

  return (
    <section aria-label="Value" className="flex min-h-0 flex-col border-l border-border">
      <header className="flex items-center gap-2 border-b border-border px-3 py-2">
        <h2 className="min-w-0 flex-1 truncate font-mono text-sm select-text">{valueKey}</h2>
        {loaded.state === "ready" && !loaded.value.truncated && !editing ? (
          <>
            <CopyButton
              label="Copy value"
              copy={() => backend.clipboard.writeText(loaded.value.value)}
            />
            <Button
              size="sm"
              disabled={busy}
              onClick={() => setEditing({ type: loaded.value.type, text: loaded.value.value })}
            >
              Edit
            </Button>
          </>
        ) : null}
        {confirming ? (
          <Button size="sm" variant="destructive" disabled={busy} onClick={() => void remove()}>
            Confirm delete
          </Button>
        ) : (
          <Button size="sm" disabled={busy} onClick={() => setConfirming(true)}>
            Delete
          </Button>
        )}
      </header>
      <div className="min-h-0 flex-1 overflow-auto p-3 text-xs">
        {loaded.state === "loading" ? (
          <p className="text-muted-foreground">Loading…</p>
        ) : loaded.state === "error" ? (
          <p role="alert" className="text-destructive">
            {loaded.message}
          </p>
        ) : loaded.value.truncated ? (
          <p className="text-muted-foreground">
            This value is {formatBytes(loaded.value.size ?? 0)}, larger than Lens shows (1 MiB).
          </p>
        ) : editing ? (
          <Editor
            draft={editing}
            onChange={setEditing}
            error={draftError}
            busy={busy}
            onSave={() => void save(loaded.value)}
            onCancel={() => {
              setEditing(null);
              setDraftError(null);
            }}
          />
        ) : (
          <Views value={loaded.value} view={view} onView={setView} />
        )}
      </div>
    </section>
  );
}

function Views({
  value,
  view,
  onView,
}: {
  value: StorageValue;
  view: View | null;
  onView: (v: View) => void;
}) {
  const json = jsonOf(value);
  const views: View[] = json.ok ? ["Text", "JSON", "Hex", "Base64"] : ["Text", "Hex", "Base64"];
  const current: View =
    view && views.includes(view) ? view : value.type === "bytes" ? "Hex" : "Text";
  return (
    <>
      <div className="mb-2 flex items-center gap-2">
        <div role="tablist" aria-label="Value views" className="flex gap-1">
          {views.map((v) => (
            <button
              key={v}
              type="button"
              role="tab"
              aria-selected={current === v}
              onClick={() => onView(v)}
              className={cn(
                "h-6 cursor-pointer rounded-md px-2 text-xs transition-colors duration-150",
                current === v
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:bg-muted/60",
              )}
            >
              {v}
            </button>
          ))}
        </div>
        <span className="text-muted-foreground">
          {formatBytes(valueSize(value))} · {value.type}
        </span>
      </div>
      <div role="tabpanel" aria-label={current}>
        {current === "JSON" && json.ok ? (
          <JsonTree value={json.value} label="Stored JSON" />
        ) : (
          <pre className="font-mono break-all whitespace-pre-wrap select-text">
            {current === "Hex"
              ? hexDump(value)
              : current === "Base64"
                ? toBase64(value)
                : value.type === "bytes"
                  ? "(binary: use the Hex or Base64 view)"
                  : value.value}
          </pre>
        )}
      </div>
    </>
  );
}

const TYPES: StorageValueType[] = ["string", "number", "boolean", "bytes"];

function Editor({
  draft,
  onChange,
  error,
  busy,
  onSave,
  onCancel,
}: {
  draft: { type: StorageValueType; text: string };
  onChange: (d: { type: StorageValueType; text: string }) => void;
  error: string | null;
  busy: boolean;
  onSave: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label className="flex items-center gap-2 text-muted-foreground">
        Type
        <select
          aria-label="Type"
          value={draft.type}
          onChange={(e) => onChange({ ...draft, type: e.target.value as StorageValueType })}
          className="h-7 rounded-md border border-border bg-background px-1 text-xs"
        >
          {TYPES.map((t) => (
            <option key={t}>{t}</option>
          ))}
        </select>
        {draft.type === "bytes" ? <span>(base64)</span> : null}
      </label>
      <textarea
        aria-label="Value"
        value={draft.text}
        onChange={(e) => onChange({ ...draft, text: e.target.value })}
        spellCheck={false}
        rows={12}
        className="w-full rounded-md border border-border bg-background p-2 font-mono text-xs outline-none focus-visible:outline-2 focus-visible:outline-ring"
      />
      {error ? (
        <p role="alert" className="text-destructive">
          {error}
        </p>
      ) : null}
      <div className="flex gap-1">
        <Button size="sm" variant="primary" disabled={busy} onClick={onSave}>
          Save to app
        </Button>
        <Button size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
