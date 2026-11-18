import { useEffect, useState, type ReactNode } from "react";

import { useBackend, type McpCall, type McpToolKind, type MCPInfo } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { errorMessage } from "@/shared/lib/errorMessage";
import { Button } from "@/shared/ui/atoms/Button";
import { CopyButton } from "@/shared/ui/molecules/CopyButton";

import { cursorConfig, mcpCommand } from "./command";
import { pullMcp, useMcpStore } from "./mcpStore";
import { useMcpSync } from "./useMcpSync";

/** Connect AI clients, watch what they do, and decide whether they may change the app. */
export function McpPanel() {
  useMcpSync();
  const status = useMcpStore((s) => s.status);
  const error = useMcpStore((s) => s.error);

  if (status && !status.enabled) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-sm text-muted-foreground">
        <p className="font-medium text-foreground">MCP is off.</p>
        <p>
          Lens was started with <code className="font-mono">--mcp-port 0</code>, or the port was
          taken by another Lens. Restart Lens without it to let AI clients connect.
        </p>
      </div>
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-auto p-4">
      <Header />
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Connect />
        <Tools />
      </div>
      <Activity />
    </div>
  );
}

function Header() {
  const backend = useBackend();
  const status = useMcpStore((s) => s.status);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const readOnly = status?.readOnly ?? false;

  const toggle = async () => {
    setBusy(true);
    setError(null);
    try {
      await backend.mcp.setReadOnly(!readOnly);
      // Refresh before the switch takes another click: the change notice can
      // arrive a tick later, and a second click would resend the old value.
      await pullMcp(backend);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <header className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 className="text-base font-semibold">MCP</h2>
        <p className="text-xs text-muted-foreground">
          Claude and other AI clients read and manage the attached app through Lens.
        </p>
      </div>
      <div className="flex flex-col items-end gap-1">
        <button
          type="button"
          role="switch"
          aria-checked={readOnly}
          aria-label="Read-only: refuse the tools that change the app"
          disabled={busy || !status}
          onClick={() => void toggle()}
          className="flex cursor-pointer items-center gap-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span
            aria-hidden="true"
            className={cn(
              "relative h-5 w-9 rounded-full transition-colors duration-150",
              readOnly ? "bg-accent" : "bg-muted",
            )}
          >
            <span
              className={cn(
                "absolute top-0.5 left-0.5 size-4 rounded-full bg-foreground transition-transform duration-150",
                readOnly ? "translate-x-4" : "translate-x-0",
              )}
            />
          </span>
          Read-only
        </button>
        <span className="text-xs text-muted-foreground">
          {readOnly ? "AI clients can only read." : "AI clients can change the app."}
        </span>
        {error ? (
          <span role="alert" className="text-xs text-destructive">
            {error}
          </span>
        ) : null}
      </div>
    </header>
  );
}

function Card({
  title,
  children,
  action,
  className,
}: {
  title: string;
  children: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <section
      aria-label={title}
      className={cn(
        "flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-card p-3 text-sm",
        className,
      )}
    >
      <header className="flex items-center justify-between gap-2">
        <h3 className="font-medium">{title}</h3>
        {action}
      </header>
      {children}
    </section>
  );
}

function Connect() {
  const backend = useBackend();
  const status = useMcpStore((s) => s.status);
  // The token is only in the copied text, never on screen.
  const [info, setInfo] = useState<MCPInfo | null>(null);
  useEffect(() => {
    let cancelled = false;
    backend.app
      .mcp()
      .then((v) => {
        if (!cancelled) setInfo(v);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [backend]);
  const url = status?.url ?? "";
  const copy = (text: string) => () => backend.clipboard.writeText(text);

  return (
    <Card title="Connect">
      <p className="font-mono text-xs break-all select-text">{url}</p>
      {status?.tokenSet ? (
        <p className="text-xs text-muted-foreground">
          Clients send a bearer token; the copied snippets include it.
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <CopyButton
          label="Copy the Claude Code command"
          text="Claude Code"
          disabled={!info}
          copy={copy(info ? mcpCommand(info) : "")}
        />
        <CopyButton
          label="Copy the Cursor configuration"
          text="Cursor (mcp.json)"
          disabled={!info}
          copy={copy(info ? cursorConfig(info) : "")}
        />
        <CopyButton label="Copy the URL" text="URL" disabled={!url} copy={copy(url)} />
      </div>
      <p className="text-xs text-muted-foreground">
        Any client that speaks MCP over streamable HTTP connects to the URL.
      </p>
    </Card>
  );
}

const kindClass: Record<McpToolKind, string> = {
  read: "text-muted-foreground",
  change: "text-info",
  destructive: "text-destructive",
};

function Tools() {
  const tools = useMcpStore((s) => s.tools);
  return (
    <Card title="Tools">
      <ul className="max-h-64 overflow-auto text-xs">
        {tools.map((t) => (
          <li
            key={t.name}
            className="flex gap-2 border-b border-border py-1 last:border-b-0"
            title={t.description}
          >
            <span className="w-36 shrink-0 font-mono">{t.name}</span>
            <span className={cn("w-20 shrink-0", kindClass[t.kind])}>{t.kind}</span>
            <span className="min-w-0 truncate text-muted-foreground">{t.description}</span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

function Activity() {
  const backend = useBackend();
  const calls = useMcpStore((s) => s.calls);
  const [open, setOpen] = useState<number | null>(null);
  const newest = [...calls].reverse();

  return (
    <Card
      title="Activity"
      // Fills the rest of the panel; only the list inside scrolls.
      className="min-h-64 flex-1"
      action={
        <Button
          size="sm"
          disabled={calls.length === 0}
          onClick={() =>
            void backend.mcp
              .clearActivity()
              .catch((err: unknown) => useMcpStore.getState().setError(errorMessage(err)))
          }
        >
          Clear
        </Button>
      }
    >
      {newest.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          No AI tool calls yet. Connect a client and ask it about your app.
        </p>
      ) : (
        <div data-testid="activity-scroll" className="min-h-0 flex-1 overflow-auto">
          <table aria-label="Tool calls" className="w-full text-left text-xs">
            <thead className="sticky top-0 bg-card text-muted-foreground">
              <tr>
                <th scope="col" className="py-1 pr-2 font-medium">
                  Time
                </th>
                <th scope="col" className="py-1 pr-2 font-medium">
                  Tool
                </th>
                <th scope="col" className="py-1 pr-2 font-medium">
                  Client
                </th>
                <th scope="col" className="py-1 pr-2 font-medium">
                  Outcome
                </th>
                <th scope="col" className="py-1 font-medium">
                  Time taken
                </th>
              </tr>
            </thead>
            <tbody>
              {newest.map((call) => (
                <CallRow
                  key={call.seq}
                  call={call}
                  open={open === call.seq}
                  onToggle={() => setOpen(open === call.seq ? null : call.seq)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

function outcome(call: McpCall): { label: string; className: string } {
  if (call.refused) return { label: "refused", className: "text-warning" };
  if (call.error) return { label: "error", className: "text-destructive" };
  return { label: "ok", className: "text-accent" };
}

function CallRow({ call, open, onToggle }: { call: McpCall; open: boolean; onToggle: () => void }) {
  const out = outcome(call);
  return (
    <>
      <tr className="border-t border-border align-top">
        <td className="py-1 pr-2 whitespace-nowrap text-muted-foreground">
          <time dateTime={call.time}>{new Date(call.time).toLocaleTimeString()}</time>
        </td>
        <td className="py-1 pr-2">
          <button
            type="button"
            aria-expanded={open}
            onClick={onToggle}
            className={cn("cursor-pointer font-mono hover:underline", kindClass[call.kind])}
          >
            {call.tool}
          </button>
        </td>
        <td className="py-1 pr-2 text-muted-foreground">{call.client || "—"}</td>
        <td className={cn("py-1 pr-2", out.className)}>{out.label}</td>
        <td className="py-1 text-muted-foreground">{call.durationMs.toFixed(1)} ms</td>
      </tr>
      {open ? (
        <tr>
          <td colSpan={5} className="pb-2">
            <div className="grid max-h-80 gap-2 overflow-auto rounded-md bg-muted/40 p-2 font-mono break-all whitespace-pre-wrap select-text">
              <div>
                <div className="text-muted-foreground">Arguments</div>
                {call.args === undefined ? "(none)" : JSON.stringify(call.args, null, 2)}
              </div>
              <div>
                <div className="text-muted-foreground">{call.error ? "Error" : "Result"}</div>
                {call.error ?? call.result ?? ""}
              </div>
            </div>
          </td>
        </tr>
      ) : null}
    </>
  );
}
