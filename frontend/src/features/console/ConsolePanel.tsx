import { Ban, Search } from "lucide-react";
import { useEffect, useMemo, useRef } from "react";

import { useBackend, type Backend, type ConsoleLevel } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";

import { ConsoleList } from "./ConsoleList";
import { countByLevel, runConsoleAction, useConsoleStore, visibleEntries } from "./consoleStore";
import { EvalBar } from "./EvalBar";
import { levelLabel } from "./presentation";
import { useConsoleSync } from "./useConsoleSync";

const LEVELS: ConsoleLevel[] = ["error", "warning", "info", "verbose", "system"];

function clearConsole(backend: Backend): void {
  void runConsoleAction(() => backend.console.clear());
}

/** Container: wires sync, filters and actions to the presentational pieces. */
export function ConsolePanel() {
  const backend = useBackend();
  useConsoleSync();
  const entries = useConsoleStore((s) => s.entries);
  const levels = useConsoleStore((s) => s.levels);
  const query = useConsoleStore((s) => s.query);
  const dropped = useConsoleStore((s) => s.dropped);
  const preserveLog = useConsoleStore((s) => s.preserveLog);
  const error = useConsoleStore((s) => s.error);
  const setQuery = useConsoleStore((s) => s.setQuery);
  const toggleLevel = useConsoleStore((s) => s.toggleLevel);
  const setPreserveLog = useConsoleStore((s) => s.setPreserveLog);
  const searchRef = useRef<HTMLInputElement>(null);

  const visible = useMemo(() => visibleEntries(entries, levels, query), [entries, levels, query]);
  const counts = useMemo(() => countByLevel(entries), [entries]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === "l") {
        e.preventDefault();
        clearConsole(backend);
      } else if (mod && e.key.toLowerCase() === "f") {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [backend]);

  return (
    <section aria-labelledby="panel-console-title" className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-2 border-b border-border px-3">
        <h1 id="panel-console-title" className="mr-2 text-sm font-semibold">
          Console
        </h1>
        <Button
          size="sm"
          aria-label="Clear console (⌘L)"
          title="Clear console (⌘L)"
          onClick={() => clearConsole(backend)}
        >
          <Ban aria-hidden="true" className="size-3.5" />
        </Button>
        <label className="relative flex min-w-0 flex-1 items-center">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute left-2 size-3.5 text-muted-foreground"
          />
          <input
            ref={searchRef}
            type="search"
            aria-label="Filter console output"
            placeholder="Filter (⌘F)"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") setQuery("");
            }}
            className="h-7 w-full rounded-md border border-border bg-background pr-2 pl-7 text-sm outline-none focus-visible:outline-2 focus-visible:outline-ring"
          />
        </label>
        <div role="group" aria-label="Levels" className="flex items-center gap-1">
          {LEVELS.map((level) => (
            <button
              key={level}
              type="button"
              aria-pressed={levels[level]}
              onClick={() => toggleLevel(level)}
              className={cn(
                "h-7 cursor-pointer rounded-md px-2 text-xs transition-colors duration-150",
                levels[level]
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:bg-muted/60",
              )}
            >
              {levelLabel[level]}
              <span className="ml-1 tabular-nums opacity-70">{counts[level]}</span>
            </button>
          ))}
        </div>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={preserveLog}
            onChange={(e) => setPreserveLog(e.target.checked)}
          />
          Preserve log
        </label>
      </header>

      {dropped > 0 ? (
        <p className="border-b border-border bg-muted/40 px-3 py-1 text-xs text-muted-foreground">
          {dropped.toLocaleString()} older entries were dropped to stay within the buffer.
        </p>
      ) : null}
      {error ? (
        <p
          role="alert"
          className="border-b border-destructive/60 bg-destructive/10 px-3 py-1 text-xs"
        >
          {error}
        </p>
      ) : null}

      {visible.length === 0 ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          {entries.length === 0
            ? "Nothing logged yet. Attach to an app to see its console."
            : "No entries match the current filters."}
        </div>
      ) : (
        <ConsoleList entries={visible} />
      )}
      <EvalBar />
    </section>
  );
}
