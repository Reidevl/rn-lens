import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { useBackend, type ConsoleProperty, type RemoteObject } from "@/backend";
import { cn } from "@/shared/lib/cn";

import { errorMessage } from "./consoleStore";
import { isExpandable } from "./presentation";

interface Props {
  object: RemoteObject;
  epoch: number;
  /** Shown as the collapsed label; defaults to the object's description. */
  label?: string;
  depth?: number;
}

/**
 * Lazily expanded remote object. Children are fetched through the Backend on
 * first expand; handles from an older epoch show a stale notice instead.
 */
export function ObjectTree({ object, epoch, label, depth = 0 }: Props) {
  const backend = useBackend();
  const [open, setOpen] = useState(false);
  const [props, setProps] = useState<ConsoleProperty[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const summary = label ?? object.description ?? object.className ?? object.type;
  if (!isExpandable(object)) {
    return <span className="font-mono">{summary}</span>;
  }

  const toggle = async () => {
    const next = !open;
    setOpen(next);
    if (next && props === null && !loading) {
      setLoading(true);
      try {
        setProps(await backend.console.properties(object.objectId!, epoch));
        setError(null);
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setLoading(false);
      }
    }
  };

  return (
    <span className="inline-block align-top">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`${open ? "Collapse" : "Expand"} ${summary}`}
        onClick={() => void toggle()}
        className="inline-flex cursor-pointer items-center gap-0.5 rounded-sm font-mono hover:bg-muted"
      >
        <ChevronRight
          aria-hidden="true"
          className={cn("size-3 transition-transform duration-150", open && "rotate-90")}
        />
        <span>{summary}</span>
      </button>
      {open ? (
        <ul
          className="border-l border-border pl-2"
          style={{ marginLeft: `${Math.min(depth, 6) * 0.25 + 1}rem` }}
        >
          {loading ? <li className="text-xs text-muted-foreground">Loading…</li> : null}
          {error ? (
            <li role="note" className="text-xs text-muted-foreground italic">
              {error}
            </li>
          ) : null}
          {props?.map((p) => (
            // Own and inherited properties may share a name (shadowed fields,
            // `constructor`), so the key carries both.
            <li
              key={`${p.isOwn ? "own" : "proto"}:${p.name}`}
              className="flex gap-1 font-mono text-xs"
            >
              <span className="text-info">{p.name}:</span>
              {p.value && isExpandable(p.value) ? (
                <ObjectTree object={p.value} epoch={epoch} label={p.summary} depth={depth + 1} />
              ) : (
                <span className={cn(p.hasGetter && "text-muted-foreground")}>{p.summary}</span>
              )}
            </li>
          ))}
          {props && props.length === 0 && !loading && !error ? (
            <li className="text-xs text-muted-foreground">No own properties</li>
          ) : null}
        </ul>
      ) : null}
    </span>
  );
}
