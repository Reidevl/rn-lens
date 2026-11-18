import { useState } from "react";

import { useBackend, type QueryActionType, type QueryItem } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";
import { RelativeTime } from "@/shared/ui/atoms/RelativeTime";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

import { keyText, labelDot, labelText, statusBadge } from "./presentation";
import { runQueriesAction, useQueriesStore } from "./queriesStore";

const ACTIONS: Array<{ type: QueryActionType; label: string; danger?: boolean }> = [
  { type: "REFETCH", label: "Refetch" },
  { type: "INVALIDATE", label: "Invalidate" },
  { type: "RESET", label: "Reset" },
  { type: "REMOVE", label: "Remove", danger: true },
  { type: "TRIGGER_LOADING", label: "Trigger loading" },
  { type: "RESTORE_LOADING", label: "Restore loading" },
  { type: "TRIGGER_ERROR", label: "Trigger error" },
  { type: "RESTORE_ERROR", label: "Restore error" },
];

export function QueryDetail({ query, deviceKey }: { query: QueryItem; deviceKey: string }) {
  const backend = useBackend();
  const busy = useQueriesStore((s) => s.busy);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [draftError, setDraftError] = useState<string | null>(null);

  const run = (type: QueryActionType, dataJSON = "") =>
    void runQueriesAction(backend, () =>
      backend.queries.action(deviceKey, type, query.queryHash, dataJSON),
    );

  const startEdit = () => {
    setDraft(JSON.stringify(query.state.data ?? null, null, 2));
    setDraftError(null);
    setEditing(true);
  };
  const saveEdit = () => {
    try {
      JSON.parse(draft);
    } catch (err) {
      setDraftError(err instanceof Error ? err.message : "Invalid JSON");
      return;
    }
    setEditing(false);
    run("SET_QUERY_DATA", draft);
  };

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="border-b border-border p-3">
        <p className="font-mono text-sm break-all select-text">{keyText(query.queryKey)}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
          <span className="flex items-center gap-1.5">
            <span aria-hidden="true" className={cn("size-2 rounded-full", labelDot[query.label])} />
            {labelText[query.label]}
          </span>
          <span className={cn("rounded-sm px-1.5 py-0.5", statusBadge(query.state.status))}>
            {query.state.status}
          </span>
          <span className="text-muted-foreground">fetch: {query.state.fetchStatus}</span>
          <span className="text-muted-foreground">observers: {query.observersCount}</span>
          <span className="text-muted-foreground">
            updated <RelativeTime ms={query.state.dataUpdatedAt} />
          </span>
          {query.state.isInvalidated ? <span className="text-warning">invalidated</span> : null}
          {query.dataTruncated ? (
            <span className="text-warning">data truncated at 1 MiB</span>
          ) : null}
        </div>
        <div role="group" aria-label="Query actions" className="mt-3 flex flex-wrap gap-1">
          {ACTIONS.map((a) => (
            <Button
              key={a.type}
              size="sm"
              variant={a.danger ? "destructive" : "ghost"}
              disabled={busy}
              onClick={() => run(a.type)}
            >
              {a.label}
            </Button>
          ))}
        </div>
      </div>

      <section aria-labelledby="query-data-title" className="flex-1 p-3">
        <div className="mb-2 flex items-center justify-between">
          <h2
            id="query-data-title"
            className="text-xs font-medium tracking-wide text-muted-foreground uppercase"
          >
            Data
          </h2>
          {editing ? (
            <div className="flex gap-1">
              <Button size="sm" variant="primary" disabled={busy} onClick={saveEdit}>
                Save to app
              </Button>
              <Button size="sm" onClick={() => setEditing(false)}>
                Cancel
              </Button>
            </div>
          ) : (
            <Button size="sm" disabled={busy} onClick={startEdit}>
              Edit data
            </Button>
          )}
        </div>
        {editing ? (
          <div>
            <textarea
              aria-label="Query data JSON"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              spellCheck={false}
              rows={12}
              className="w-full rounded-md border border-border bg-background p-2 font-mono text-xs outline-none focus-visible:outline-2 focus-visible:outline-ring"
            />
            {draftError ? (
              <p role="alert" className="mt-1 text-xs text-destructive">
                {draftError}
              </p>
            ) : null}
          </div>
        ) : (
          <JsonTree value={query.state.data ?? null} label="Query data" />
        )}
        {query.state.error != null ? (
          <div className="mt-4">
            <h3 className="mb-1 text-xs font-medium tracking-wide text-destructive uppercase">
              Error
            </h3>
            <JsonTree value={query.state.error} label="Query error" />
          </div>
        ) : null}
      </section>
    </div>
  );
}
