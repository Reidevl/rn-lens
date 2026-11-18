import { useBackend } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";
import { RelativeTime } from "@/shared/ui/atoms/RelativeTime";
import { CopyButton } from "@/shared/ui/molecules/CopyButton";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

import { DevicePicker } from "./DevicePicker";
import { NoSdk } from "./NoSdk";
import { jsonText, keyText, statusBadge } from "./presentation";
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
  const copy = (value: unknown) => () => backend.clipboard.writeText(jsonText(value));
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
                      <RelativeTime ms={m.state.submittedAt} />
                    </span>
                  </button>
                </li>
              ))
            )}
          </ul>
          <div className="min-h-0 overflow-auto p-3">
            {selected ? (
              <div className="flex flex-col gap-4">
                <div className="flex justify-end">
                  <CopyButton label="Copy mutation" copy={copy(selected)} />
                </div>
                <JsonSection
                  title="Variables"
                  label="Mutation variables"
                  value={selected.state.variables}
                  copy={copy(selected.state.variables)}
                />
                <JsonSection
                  title="Data"
                  label="Mutation data"
                  value={selected.state.data}
                  copy={copy(selected.state.data)}
                />
                {selected.state.error != null ? (
                  <JsonSection
                    title="Error"
                    label="Mutation error"
                    value={selected.state.error}
                    copy={copy(selected.state.error)}
                    danger
                  />
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

function JsonSection({
  title,
  label,
  value,
  copy,
  danger,
}: {
  title: string;
  label: string;
  value: unknown;
  copy: () => Promise<void>;
  danger?: boolean;
}) {
  return (
    <div>
      <div className="mb-1 flex items-center justify-between">
        <h2
          className={cn(
            "text-xs font-medium tracking-wide uppercase",
            danger ? "text-destructive" : "text-muted-foreground",
          )}
        >
          {title}
        </h2>
        <CopyButton label={`Copy ${title.toLowerCase()}`} copy={copy} />
      </div>
      <JsonTree value={value ?? null} label={label} />
    </div>
  );
}
