import { AlertTriangle, ChevronRight, CircleX, Info, Terminal } from "lucide-react";
import { useState } from "react";

import type { ConsoleEntry, StackTrace } from "@/backend";
import { cn } from "@/shared/lib/cn";

import { ObjectTree } from "./ObjectTree";
import { formatTime, isExpandable, rowClasses, shortUrl } from "./presentation";

function Glyph({ entry }: { entry: ConsoleEntry }) {
  const cls = "size-3.5 shrink-0";
  if (entry.kind === "command")
    return <span className="w-3.5 shrink-0 text-center text-muted-foreground">›</span>;
  if (entry.kind === "result")
    return <span className="w-3.5 shrink-0 text-center text-muted-foreground">‹</span>;
  switch (entry.level) {
    case "error":
      return <CircleX aria-label="error" className={cls} />;
    case "warning":
      return <AlertTriangle aria-label="warning" className={cls} />;
    case "info":
      return entry.kind === "info" ? (
        <Info aria-label="info" className={cls} />
      ) : (
        <span className="w-3.5 shrink-0" />
      );
    case "system":
      return <Terminal aria-label="system" className={cls} />;
    default:
      return <span className="w-3.5 shrink-0" />;
  }
}

function StackFrames({ stack }: { stack: StackTrace }) {
  const [open, setOpen] = useState(false);
  const frames = stack.callFrames;
  if (frames.length === 0) return null;
  return (
    <div className="mt-0.5">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="inline-flex cursor-pointer items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
      >
        <ChevronRight
          aria-hidden="true"
          className={cn("size-3 transition-transform duration-150", open && "rotate-90")}
        />
        {frames.length} frame{frames.length === 1 ? "" : "s"}
      </button>
      {open ? (
        <ol className="ml-4 font-mono text-xs text-muted-foreground">
          {frames.map((f, i) => (
            <li key={i}>
              {f.functionName || "(anonymous)"} @ {shortUrl(f.url)}:{f.lineNumber}:{f.columnNumber}
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  );
}

/** One console line. Object arguments expand lazily; stacks are collapsed. */
export function ConsoleRow({ entry }: { entry: ConsoleEntry }) {
  if (entry.kind === "system") {
    return (
      <div className="flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        <span>{entry.text}</span>
        <span className="h-px flex-1 bg-border" />
      </div>
    );
  }

  const objectArgs = (entry.args ?? []).filter(isExpandable);
  const showArgs = objectArgs.length > 0 && entry.kind !== "command";

  return (
    <div
      className={cn(
        "flex gap-2 border-b border-border/40 px-3 py-1 font-mono text-sm",
        rowClasses(entry),
      )}
    >
      <span className="w-[5.5rem] shrink-0 text-xs text-muted-foreground tabular-nums">
        {formatTime(entry.timestamp)}
      </span>
      <Glyph entry={entry} />
      <div className="min-w-0 flex-1">
        {showArgs ? (
          <div className="flex flex-wrap gap-x-2 gap-y-0.5 break-words whitespace-pre-wrap">
            {(entry.args ?? []).map((arg, i) =>
              isExpandable(arg) ? (
                <ObjectTree key={i} object={arg} epoch={entry.epoch} />
              ) : (
                <span key={i}>
                  {arg.type === "string"
                    ? String(arg.value ?? "")
                    : (arg.description ?? String(arg.value ?? arg.type))}
                </span>
              ),
            )}
          </div>
        ) : (
          <div className="break-words whitespace-pre-wrap">{entry.text}</div>
        )}
        {entry.stack && (entry.level === "error" || entry.kind === "trace") ? (
          <StackFrames stack={entry.stack} />
        ) : null}
      </div>
      {entry.location ? (
        <span
          className="max-w-[14rem] shrink-0 truncate text-xs text-muted-foreground"
          title={entry.location.url}
        >
          {shortUrl(entry.location.url)}:{entry.location.line}
        </span>
      ) : null}
    </div>
  );
}
