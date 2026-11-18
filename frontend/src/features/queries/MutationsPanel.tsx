import { useBackend } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { useNow } from "@/shared/lib/useNow";
import { Button } from "@/shared/ui/atoms/Button";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

import { DevicePicker } from "./DevicePicker";
import { NoSdk } from "./NoSdk";
import { ago, keyText, statusBadge } from "./presentation";
import { runQueriesAction, useQueriesStore } from "./queriesStore";
import { useQueriesSync } from "./useQueriesSync";

export function MutationsPanel() {
  const backend = useBackend();
  useQueriesSync();
  const devices = useQueriesStore((s) => s.devices);
  const deviceKey = useQueriesStore((s) => s.deviceKey);
  const mutations = useQueriesStore((s) => s.mutations);
  const selectedId = useQueriesStore((s) => s.selectedMutation);
  const selectMutation = useQueriesStore((s) => s.selectMutation);
  const busy = useQueriesStore((s) => s.busy);
  const error = useQueriesStore((s) => s.error);
  const now = useNow();
  const selected = mutations.find((m) => m.mutationId === selectedId) ?? null;

  return (
    <section aria-labelledby="panel-mutations-title" className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-3 border-b border-border px-3">
        <h1 id="panel-mutations-title" className="text-sm font-semibold">
          Mutations
        </h1>
        <DevicePicker devices={devices} deviceKey={deviceKey} />
        <div className="flex-1" />
        {deviceKey ? (
          <Button
            size="sm"
            disabled={busy}
            onClick={() =>
              void runQueriesAction(backend, () =>
                backend.queries.action(deviceKey, "CLEAR_MUTATION_CACHE", "", ""),
              )
            }
          >
            Clear mutation cache
          </Button>
        ) : null}
      </header>
      {error ? (
        <p
          role="alert"
          className="border-b border-destructive/60 bg-destructive/10 px-3 py-1 text-xs"
        >
          {error}
        </p>
      ) : null}
      {!deviceKey ? (
        <NoSdk />
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(18rem,2fr)_3fr]">
          <ul
            role="listbox"
            aria-label="Mutations"
            className="min-h-0 overflow-auto border-r border-border"
          >
            {mutations.length === 0 ? (
              <li className="p-4 text-sm text-muted-foreground">No mutations have run yet.</li>
            ) : (
              mutations.map((m) => (
                <li key={m.mutationId}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={m.mutationId === selectedId}
                    onClick={() => selectMutation(m.mutationId)}
                    className={cn(
                      "flex w-full cursor-pointer items-center gap-2 border-b border-border/40 px-3 py-1.5 text-left text-sm",
                      m.mutationId === selectedId ? "bg-muted" : "hover:bg-muted/50",
                    )}
                  >
                    <span
                      className={cn(
                        "shrink-0 rounded-sm px-1.5 py-0.5 text-xs",
                        statusBadge(m.state.status),
                      )}
                    >
                      {m.state.isPaused ? "paused" : m.state.status}
                    </span>
                    <span className="min-w-0 flex-1 truncate font-mono text-xs">
                      {m.mutationKey ? keyText(m.mutationKey) : `#${m.mutationId}`}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {ago(m.state.submittedAt, now)}
                    </span>
                  </button>
                </li>
              ))
            )}
          </ul>
          <div className="min-h-0 overflow-auto p-3">
            {selected ? (
              <div className="flex flex-col gap-4">
                <div>
                  <h2 className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    Variables
                  </h2>
                  <JsonTree value={selected.state.variables ?? null} label="Mutation variables" />
                </div>
                <div>
                  <h2 className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    Data
                  </h2>
                  <JsonTree value={selected.state.data ?? null} label="Mutation data" />
                </div>
                {selected.state.error != null ? (
                  <div>
                    <h2 className="mb-1 text-xs font-medium tracking-wide text-destructive uppercase">
                      Error
                    </h2>
                    <JsonTree value={selected.state.error} label="Mutation error" />
                  </div>
                ) : null}
              </div>
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                Select a mutation to inspect it.
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
