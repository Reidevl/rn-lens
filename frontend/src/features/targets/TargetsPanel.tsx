import { ExternalLink, Plug, RefreshCw, Unplug } from "lucide-react";

import { useBackend, type Target } from "@/backend";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";

import { describeApp, presentSession, type Tone } from "./sessionPresentation";
import { runAction, useTargetsStore } from "./targetsStore";

const toneDot: Record<Tone, string> = {
  neutral: "bg-muted-foreground",
  ok: "bg-accent",
  warn: "bg-warning",
  bad: "bg-destructive",
  info: "bg-info",
};

/** Container: wires the store and backend actions to the presentational pieces. */
export function TargetsPanel() {
  const backend = useBackend();
  const targets = useTargetsStore((s) => s.targets);
  const session = useTargetsStore((s) => s.session);
  const error = useTargetsStore((s) => s.error);
  const busy = useTargetsStore((s) => s.busy);

  const attachedId = session.target?.id;
  const live =
    session.state === "attached" ||
    session.state === "connecting" ||
    session.state === "reconnecting";

  return (
    <section aria-labelledby="panel-targets-title" className="flex h-full flex-col">
      <header className="flex h-12 items-center justify-between border-b border-border px-4">
        <h1 id="panel-targets-title" className="text-sm font-semibold">
          Targets
        </h1>
        <Button
          size="sm"
          aria-label="Refresh targets"
          disabled={busy}
          onClick={() => void runAction(backend, () => backend.targets.refresh())}
        >
          <RefreshCw aria-hidden="true" className="size-3.5" />
          Refresh
        </Button>
      </header>

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
        {error ? (
          <p
            role="alert"
            className="rounded-md border border-destructive/60 bg-destructive/10 px-3 py-2 text-sm"
          >
            {error}
          </p>
        ) : null}

        {live || session.state === "evicted" ? (
          <SessionCard
            onDetach={() => void runAction(backend, () => backend.targets.detach())}
            onReload={() => void runAction(backend, () => backend.targets.reload())}
            onReattach={() =>
              session.target
                ? void runAction(backend, () => backend.targets.attach(session.target!.id))
                : undefined
            }
            busy={busy}
          />
        ) : null}

        <div>
          <h2 className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Discovered apps
          </h2>
          {targets.length === 0 ? (
            <EmptyTargets />
          ) : (
            <ul className="flex flex-col gap-2">
              {targets.map((target) => (
                <TargetRow
                  key={target.id}
                  target={target}
                  attached={target.id === attachedId && live}
                  busy={busy}
                  onAttach={() => void runAction(backend, () => backend.targets.attach(target.id))}
                  onOpenDevTools={() =>
                    void runAction(backend, () =>
                      backend.targets.openInReactNativeDevTools(target.id),
                    )
                  }
                />
              ))}
            </ul>
          )}
        </div>
      </div>
    </section>
  );
}

function EmptyTargets() {
  return (
    <div className="rounded-md border border-dashed border-border p-6 text-sm text-muted-foreground">
      <p className="font-medium text-foreground">No React Native app found yet.</p>
      <p className="mt-1">
        Start your app with Metro running (<code>npx expo start</code> or{" "}
        <code>npx react-native start</code>). Lens polls <code>localhost:8081</code>,{" "}
        <code>8082</code> and <code>19000</code> every second.
      </p>
    </div>
  );
}

function TargetRow({
  target,
  attached,
  busy,
  onAttach,
  onOpenDevTools,
}: {
  target: Target;
  attached: boolean;
  busy: boolean;
  onAttach: () => void;
  onOpenDevTools: () => void;
}) {
  const exclusive = !target.capabilities.supportsMultipleDebuggers;
  return (
    <li
      className={cn(
        "flex items-center justify-between gap-3 rounded-md border border-border bg-card px-3 py-2",
        attached && "border-accent/60",
      )}
    >
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{target.title || target.appId}</p>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {target.appId} · {target.deviceName || "unknown device"} ·{" "}
          {target.endpoint.replace(/^https?:\/\//, "")}
        </p>
        <div className="mt-1 flex flex-wrap gap-1">
          {target.reloadable ? <Badge>reloadable</Badge> : <Badge tone="warn">legacy page</Badge>}
          {exclusive ? <Badge tone="warn">exclusive attach</Badge> : <Badge>multi-debugger</Badge>}
          {target.vm ? <Badge>{target.vm}</Badge> : null}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button
          size="sm"
          aria-label={`Open ${target.title || target.appId} in React Native DevTools`}
          disabled={busy}
          onClick={onOpenDevTools}
        >
          <ExternalLink aria-hidden="true" className="size-3.5" />
        </Button>
        <Button
          size="sm"
          variant={attached ? "ghost" : "primary"}
          aria-label={
            attached
              ? `Attached to ${target.title || target.appId}`
              : `Attach to ${target.title || target.appId}`
          }
          disabled={busy || attached}
          onClick={onAttach}
        >
          <Plug aria-hidden="true" className="size-3.5" />
          {attached ? "Attached" : "Attach"}
        </Button>
      </div>
    </li>
  );
}

function SessionCard({
  onDetach,
  onReload,
  onReattach,
  busy,
}: {
  onDetach: () => void;
  onReload: () => void;
  onReattach: () => void;
  busy: boolean;
}) {
  const session = useTargetsStore((s) => s.session);
  const view = presentSession(session);
  const md = session.metadata;
  const attached = session.state === "attached";

  return (
    <div className="rounded-md border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="flex items-center gap-2 text-sm font-medium">
            <span aria-hidden="true" className={cn("size-2 rounded-full", toneDot[view.tone])} />
            <span>{view.label}</span>
          </p>
          <p className="truncate text-sm text-muted-foreground">{describeApp(session) ?? ""}</p>
          {view.detail ? <p className="mt-1 text-xs text-muted-foreground">{view.detail}</p> : null}
        </div>
        <div className="flex shrink-0 gap-1">
          {session.state === "evicted" ? (
            <Button size="sm" variant="primary" disabled={busy} onClick={onReattach}>
              <Plug aria-hidden="true" className="size-3.5" />
              Attach again
            </Button>
          ) : (
            <>
              <Button size="sm" disabled={busy || !attached} onClick={onReload}>
                <RefreshCw aria-hidden="true" className="size-3.5" />
                Reload app
              </Button>
              <Button size="sm" disabled={busy} onClick={onDetach}>
                <Unplug aria-hidden="true" className="size-3.5" />
                Detach
              </Button>
            </>
          )}
        </div>
      </div>

      {md ? (
        <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
          <Field label="App" value={`${md.appDisplayName} (${md.appIdentifier})`} />
          <Field label="Device" value={md.deviceName} />
          <Field label="Platform" value={md.platform} />
          <Field label="React Native" value={md.reactNativeVersion} />
          <Field label="Integration" value={md.integrationName} />
          <Field
            label="Network inspection"
            value={
              md.unstable_networkInspectionEnabled
                ? "available"
                : "not available (needs React Native 0.83+)"
            }
          />
        </dl>
      ) : null}
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="truncate font-mono">{value || "—"}</dd>
    </>
  );
}

function Badge({
  children,
  tone = "neutral",
}: {
  children: React.ReactNode;
  tone?: "neutral" | "warn";
}) {
  return (
    <span
      className={cn(
        "rounded-sm px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase",
        tone === "warn" ? "bg-warning/15 text-warning" : "bg-muted text-muted-foreground",
      )}
    >
      {children}
    </span>
  );
}
