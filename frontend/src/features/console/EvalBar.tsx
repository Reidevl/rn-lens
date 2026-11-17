import { useRef, useState, type KeyboardEvent } from "react";

import { useBackend } from "@/backend";
import { useTargetsStore } from "@/features/targets/targetsStore";

import { runConsoleAction, useConsoleStore } from "./consoleStore";

/**
 * Expression input with ↑/↓ history. Disabled until an app is attached. It
 * stays enabled while an action runs (a disabled input drops focus in the
 * webview, which would break ↑/↓ right after Enter); Enter is ignored until
 * the running action settles.
 */
export function EvalBar() {
  const backend = useBackend();
  const attached = useTargetsStore((s) => s.session.state === "attached");
  const history = useConsoleStore((s) => s.history);
  const pushHistory = useConsoleStore((s) => s.pushHistory);
  const busy = useConsoleStore((s) => s.busy);
  const [value, setValue] = useState("");
  const cursor = useRef<number | null>(null);
  const draft = useRef("");

  const submit = async () => {
    const expression = value.trim();
    if (!expression || busy) return;
    pushHistory(expression);
    cursor.current = null;
    setValue("");
    await runConsoleAction(async () => {
      await backend.console.evaluate(expression);
    });
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void submit();
      return;
    }
    if (e.key === "ArrowUp" || e.key === "ArrowDown") {
      if (history.length === 0) return;
      e.preventDefault();
      if (cursor.current === null) {
        if (e.key === "ArrowDown") return;
        draft.current = value;
        cursor.current = history.length - 1;
      } else if (e.key === "ArrowUp") {
        cursor.current = Math.max(0, cursor.current - 1);
      } else if (cursor.current >= history.length - 1) {
        cursor.current = null;
        setValue(draft.current);
        return;
      } else {
        cursor.current += 1;
      }
      setValue(history[cursor.current]);
    }
  };

  return (
    <div className="flex items-center gap-2 border-t border-border bg-card px-3 py-1.5">
      <span aria-hidden="true" className="font-mono text-info">
        ›
      </span>
      <input
        type="text"
        aria-label="Evaluate expression in the app"
        placeholder={
          attached
            ? "Evaluate JavaScript in the app… (Enter to run, ↑↓ history)"
            : "Attach to an app to evaluate expressions"
        }
        disabled={!attached}
        aria-busy={busy}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={onKeyDown}
        spellCheck={false}
        autoComplete="off"
        className="h-8 min-w-0 flex-1 bg-transparent font-mono text-sm outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed"
      />
    </div>
  );
}
