import { Component, useEffect, useRef, useState, type ReactNode } from "react";

import type { PluginBlock, PluginButton } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

/** Asks the panel to send a click: block, action and the row for row actions. */
export type ActionHandler = (blockId: string, actionId: string, rowId?: string) => void;

interface BlockProps {
  block: PluginBlock;
  onAction: ActionHandler;
  /** "<blockId>/<actionId>/<rowId>" of the action in flight, if any. */
  running: string | null;
}

/** Draws one block; a malformed block shows an error instead of breaking the panel. */
export function Block(props: BlockProps) {
  return (
    <BlockBoundary block={props.block}>
      <BlockBody {...props} />
    </BlockBoundary>
  );
}

/** Contains a render error to its block; a new view of the block retries. */
class BlockBoundary extends Component<
  { block: PluginBlock; children: ReactNode },
  { error: string | null; block: PluginBlock }
> {
  state = { error: null as string | null, block: this.props.block };

  static getDerivedStateFromProps(
    props: { block: PluginBlock },
    state: { error: string | null; block: PluginBlock },
  ) {
    return props.block === state.block ? null : { error: null, block: props.block };
  }

  static getDerivedStateFromError(err: unknown) {
    return { error: err instanceof Error ? err.message : String(err) };
  }

  render() {
    if (this.state.error === null) return this.props.children;
    const { id, type } = this.props.block as { id?: unknown; type?: unknown };
    return (
      <p role="alert" className="text-xs text-destructive">
        This block could not be drawn ({String(type)} &quot;{String(id)}&quot;): {this.state.error}
      </p>
    );
  }
}

function BlockBody({ block, onAction, running }: BlockProps) {
  const body = (() => {
    switch (block.type) {
      case "text":
        return <TextBlock text={block.text} tone={block.tone} />;
      case "keyValue":
        return <KeyValueBlock items={block.items} />;
      case "table":
        return <TableBlock block={block} onAction={onAction} running={running} />;
      case "json":
        return <JsonTree value={block.value} label={block.title ?? block.id} />;
      case "log":
        return <LogBlock entries={block.entries} />;
      case "actions":
        return (
          <div className="flex flex-wrap gap-2">
            {block.actions.map((a) => (
              <ActionButton
                key={a.id}
                action={a}
                busy={running === `${block.id}/${a.id}/`}
                disabled={running !== null}
                onRun={() => onAction(block.id, a.id)}
              />
            ))}
          </div>
        );
      default:
        return (
          <p className="text-xs text-muted-foreground">
            This block ({String((block as { type?: unknown }).type)}) needs a newer version of Lens.
          </p>
        );
    }
  })();
  return (
    <section aria-label={block.title ?? block.id} className="flex flex-col gap-2">
      {block.title ? (
        <h3 className="text-xs font-medium text-muted-foreground">{block.title}</h3>
      ) : null}
      {body}
    </section>
  );
}

const toneClass = {
  default: "text-foreground",
  muted: "text-muted-foreground",
  warning: "text-warning",
  error: "text-destructive",
} as const;

function TextBlock({ text, tone = "default" }: { text: string; tone?: keyof typeof toneClass }) {
  return (
    <p
      role={tone === "error" ? "alert" : undefined}
      className={cn(
        "text-sm whitespace-pre-wrap select-text",
        toneClass[tone] ?? toneClass.default,
      )}
    >
      {text}
    </p>
  );
}

/** A primitive inline; objects and arrays as a compact JSON tree. */
function Value({ value, label }: { value: unknown; label: string }) {
  if (value !== null && typeof value === "object") return <JsonTree value={value} label={label} />;
  return <span className="font-mono text-xs break-all select-text">{format(value)}</span>;
}

function format(value: unknown): string {
  if (value === undefined || value === null) return value === null ? "null" : "";
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}

function KeyValueBlock({ items }: { items: { key: string; label?: string; value: unknown }[] }) {
  return (
    <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-1 text-sm">
      {items.map((item) => (
        <div key={item.key} className="contents">
          <dt className="text-muted-foreground">{item.label ?? item.key}</dt>
          <dd>
            <Value value={item.value} label={item.label ?? item.key} />
          </dd>
        </div>
      ))}
    </dl>
  );
}

function TableBlock({
  block,
  onAction,
  running,
}: { block: Extract<PluginBlock, { type: "table" }> } & Omit<BlockProps, "block">) {
  const actions = block.rowActions ?? [];
  return (
    <div className="overflow-auto rounded-md border border-border">
      <table aria-label={block.title ?? block.id} className="w-full text-left text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            {block.columns.map((c) => (
              <th key={c.key} scope="col" className="px-2 py-1 font-medium">
                {c.label}
              </th>
            ))}
            {actions.length > 0 ? (
              <th scope="col" className="px-2 py-1">
                <span className="sr-only">Actions</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {block.rows.length === 0 ? (
            <tr>
              <td
                colSpan={block.columns.length + 1}
                className="px-2 py-2 text-xs text-muted-foreground"
              >
                No rows.
              </td>
            </tr>
          ) : (
            block.rows.map((row) => (
              <tr key={row.id} className="border-t border-border">
                {block.columns.map((c) => (
                  <td key={c.key} className="px-2 py-1 align-top">
                    <Value value={row.cells[c.key]} label={`${row.id} ${c.label}`} />
                  </td>
                ))}
                {actions.length > 0 ? (
                  <td className="px-2 py-1 text-right whitespace-nowrap">
                    <div className="inline-flex gap-1">
                      {actions.map((a) => (
                        <ActionButton
                          key={a.id}
                          action={a}
                          context={row.id}
                          busy={running === `${block.id}/${a.id}/${row.id}`}
                          disabled={running !== null}
                          onRun={() => onAction(block.id, a.id, row.id)}
                        />
                      ))}
                    </div>
                  </td>
                ) : null}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

const levelClass = {
  debug: "text-muted-foreground",
  info: "text-info",
  warn: "text-warning",
  error: "text-destructive",
} as const;

function LogBlock({
  entries,
}: {
  entries: { text: string; level?: keyof typeof levelClass; time?: string }[];
}) {
  if (entries.length === 0) return <p className="text-xs text-muted-foreground">No entries.</p>;
  return (
    <ol className="max-h-80 overflow-auto rounded-md border border-border font-mono text-xs">
      {entries.map((e, i) => (
        <li key={i} className="flex gap-2 border-b border-border px-2 py-1 last:border-b-0">
          {e.time ? <time className="shrink-0 text-muted-foreground">{e.time}</time> : null}
          {e.level ? (
            <span className={cn("w-10 shrink-0 uppercase", levelClass[e.level] ?? "")}>
              {e.level}
            </span>
          ) : null}
          <span className="break-all whitespace-pre-wrap select-text">{e.text}</span>
        </li>
      ))}
    </ol>
  );
}

/** A plugin button; `confirm` asks before sending, Escape cancels. */
function ActionButton({
  action,
  context,
  busy,
  disabled,
  onRun,
}: {
  action: PluginButton;
  /** Disambiguates row buttons for assistive technology. */
  context?: string;
  busy: boolean;
  disabled: boolean;
  onRun: () => void;
}) {
  const [asking, setAsking] = useState(false);
  const cancel = useRef<HTMLButtonElement>(null);
  // Focus Cancel when the prompt opens, so Escape and Enter act on it.
  useEffect(() => {
    if (asking) cancel.current?.focus();
  }, [asking]);
  const name = context ? `${action.label} ${context}` : action.label;
  if (asking) {
    return (
      <span
        className="inline-flex items-center gap-1"
        onKeyDown={(e) => e.key === "Escape" && setAsking(false)}
      >
        <span className="text-xs">{action.confirm}</span>
        <Button
          size="sm"
          variant={action.destructive ? "destructive" : "primary"}
          aria-label={`Confirm ${name}`}
          onClick={() => {
            setAsking(false);
            onRun();
          }}
        >
          Confirm
        </Button>
        <Button ref={cancel} size="sm" onClick={() => setAsking(false)}>
          Cancel
        </Button>
      </span>
    );
  }
  return (
    <Button
      size="sm"
      variant={action.destructive ? "destructive" : "ghost"}
      className={action.destructive ? undefined : "border border-border"}
      aria-label={context ? name : undefined}
      disabled={disabled}
      onClick={() => (action.confirm ? setAsking(true) : onRun())}
    >
      {busy ? "…" : action.label}
    </Button>
  );
}
