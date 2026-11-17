import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { cn } from "@/shared/lib/cn";

const MAX_CHILDREN = 200;

function describe(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return `Array(${value.length})`;
  switch (typeof value) {
    case "string":
      return JSON.stringify(value);
    case "object":
      return `{…} ${Object.keys(value as object).length} keys`;
    case "undefined":
      return "undefined";
    default:
      return String(value);
  }
}

function isBranch(value: unknown): boolean {
  return typeof value === "object" && value !== null && Object.keys(value as object).length > 0;
}

function valueClass(value: unknown): string {
  if (value === null || value === undefined) return "text-muted-foreground";
  switch (typeof value) {
    case "string":
      return "text-accent";
    case "number":
    case "bigint":
      return "text-info";
    case "boolean":
      return "text-warning";
    default:
      return "text-foreground";
  }
}

interface NodeProps {
  name?: string;
  value: unknown;
  depth: number;
  defaultOpen: boolean;
}

function Node({ name, value, depth, defaultOpen }: NodeProps) {
  const [open, setOpen] = useState(defaultOpen);
  const branch = isBranch(value);
  const label = name !== undefined ? `${name}: ` : "";

  if (!branch) {
    return (
      <div className="flex gap-1 font-mono text-xs" style={{ paddingLeft: `${depth * 0.75}rem` }}>
        {name !== undefined ? <span className="text-info">{name}:</span> : null}
        <span className={cn("break-all", valueClass(value))}>{describe(value)}</span>
      </div>
    );
  }

  const entries = Object.entries(value as Record<string, unknown>);
  const shown = entries.slice(0, MAX_CHILDREN);
  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex cursor-pointer items-center gap-0.5 rounded-sm font-mono text-xs hover:bg-muted"
        style={{ paddingLeft: `${depth * 0.75}rem` }}
      >
        <ChevronRight
          aria-hidden="true"
          className={cn("size-3 transition-transform duration-150", open && "rotate-90")}
        />
        <span>
          {label}
          <span className="text-muted-foreground">{describe(value)}</span>
        </span>
      </button>
      {open ? (
        <div>
          {shown.map(([k, v]) => (
            <Node key={k} name={k} value={v} depth={depth + 1} defaultOpen={false} />
          ))}
          {entries.length > MAX_CHILDREN ? (
            <div
              className="font-mono text-xs text-muted-foreground"
              style={{ paddingLeft: `${(depth + 1) * 0.75}rem` }}
            >
              … {entries.length - MAX_CHILDREN} more
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Read-only tree for plain JSON values (the SDK already serialized them). */
export function JsonTree({ value, label }: { value: unknown; label?: string }) {
  return (
    <div role="tree" aria-label={label ?? "JSON"} className="select-text">
      <Node value={value} depth={0} defaultOpen />
    </div>
  );
}
