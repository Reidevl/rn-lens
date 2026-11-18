import { useEffect, useState, type ReactNode } from "react";

import {
  useBackend,
  type HeapSnapshotResult,
  type ProfileSummary,
  type Screenshot,
  type ScreenshotFormat,
  type TraceSummary,
} from "@/backend";
import { useTargetsStore } from "@/features/targets/targetsStore";
import { errorMessage } from "@/shared/lib/errorMessage";
import { formatBytes } from "@/shared/lib/formatBytes";
import { Button } from "@/shared/ui/atoms/Button";

import { usePerformanceStore } from "./performanceStore";
import { formatDuration, TRACE_CATEGORIES } from "./presentation";

/** Screenshot, trace, CPU profile and heap snapshot of the attached app. */
export function PerformancePanel() {
  const attached = useTargetsStore((s) => s.session.state === "attached");
  if (!attached) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-1 p-6 text-center text-sm text-muted-foreground">
        Attach to an app on the Targets panel to record its performance.
      </div>
    );
  }
  return (
    <div className="grid h-full min-h-0 auto-rows-min grid-cols-1 gap-3 overflow-auto p-3 lg:grid-cols-2">
      <ScreenshotCard />
      <TraceCard />
      <ProfileCard />
      <HeapCard />
    </div>
  );
}

/** Runs one backend action at a time and keeps its error and saved path. */
function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const run = async <T,>(action: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    setError(null);
    try {
      return await action();
    } catch (err) {
      setError(errorMessage(err));
      return undefined;
    } finally {
      setBusy(false);
    }
  };
  const save = async (action: () => Promise<string>) => {
    const path = await run(action);
    if (path) setSaved(path);
  };
  return { busy, error, saved, setSaved, run, save };
}

function Card({
  title,
  description,
  children,
  error,
  saved,
}: {
  title: string;
  description: string;
  children: ReactNode;
  error: string | null;
  saved: string | null;
}) {
  return (
    <section
      aria-label={title}
      className="flex flex-col gap-2 rounded-lg border border-border bg-card p-3 text-sm"
    >
      <header>
        <h2 className="font-medium">{title}</h2>
        <p className="text-xs text-muted-foreground">{description}</p>
      </header>
      {children}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      {saved ? (
        <p role="status" className="truncate text-xs text-muted-foreground" title={saved}>
          Saved to <span className="font-mono select-text">{saved}</span>
        </p>
      ) : null}
    </section>
  );
}

function ScreenshotCard() {
  const backend = useBackend();
  const { busy, error, saved, setSaved, run, save } = useAction();
  const [format, setFormat] = useState<ScreenshotFormat>("png");
  const [shot, setShot] = useState<Screenshot | null>(null);
  const [copied, setCopied] = useState(false);
  const src = shot ? `data:image/${shot.format};base64,${shot.data}` : null;

  const capture = async () => {
    const result = await run(() => backend.performance.screenshot(format, 90));
    if (result) {
      setShot(result);
      setSaved(null);
      setCopied(false);
    }
  };
  const copy = async () => {
    if (!src) return;
    const ok = await run(async () => {
      const blob = await (await fetch(src)).blob();
      await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
      return true;
    });
    setCopied(ok === true);
  };

  return (
    <Card
      title="Screenshot"
      description="Captures what the app shows right now."
      error={error}
      saved={saved}
    >
      <div className="flex items-center gap-2">
        <select
          aria-label="Image format"
          value={format}
          onChange={(e) => setFormat(e.target.value as ScreenshotFormat)}
          className="h-7 rounded-md border border-border bg-background px-1 text-xs"
        >
          <option value="png">PNG</option>
          <option value="jpeg">JPEG</option>
        </select>
        <Button size="sm" variant="primary" disabled={busy} onClick={() => void capture()}>
          Capture
        </Button>
        {shot ? (
          <>
            <Button
              size="sm"
              disabled={busy}
              onClick={() => void save(backend.performance.saveScreenshot)}
            >
              Save…
            </Button>
            {shot.format === "png" ? (
              <Button size="sm" disabled={busy} onClick={() => void copy()}>
                {copied ? "Copied" : "Copy image"}
              </Button>
            ) : null}
            <span className="text-xs text-muted-foreground">{formatBytes(shot.bytes)}</span>
          </>
        ) : null}
      </div>
      {src ? (
        <img
          src={src}
          alt="Screenshot of the attached app"
          className="max-h-96 self-start rounded-md border border-border object-contain"
        />
      ) : null}
    </Card>
  );
}

function TraceCard() {
  const backend = useBackend();
  const tracing = usePerformanceStore((s) => s.status.tracing);
  const { busy, error, saved, setSaved, run, save } = useAction();
  const [selected, setSelected] = useState<Set<string> | null>(null);
  const [summary, setSummary] = useState<TraceSummary | null>(null);

  useEffect(() => {
    let cancelled = false;
    backend.performance
      .traceCategories()
      .then((defaults) => {
        if (!cancelled) setSelected((cur) => cur ?? new Set(defaults));
      })
      .catch(() => {
        if (!cancelled) setSelected((cur) => cur ?? new Set());
      });
    return () => {
      cancelled = true;
    };
  }, [backend]);

  const toggle = (id: string) =>
    setSelected((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const start = async () => {
    const order = TRACE_CATEGORIES.map((c) => c.id).filter((id) => selected?.has(id));
    await run(() => backend.performance.startTrace(order));
  };
  const stop = async () => {
    const result = await run(backend.performance.stopTrace);
    if (result) {
      setSummary(result);
      setSaved(null);
    }
  };

  return (
    <Card
      title="Trace"
      description="Records a Chrome trace that opens in Perfetto UI (ui.perfetto.dev) or Chrome DevTools."
      error={error}
      saved={saved}
    >
      <fieldset disabled={tracing || busy} className="flex flex-wrap gap-x-3 gap-y-1 text-xs">
        <legend className="sr-only">Categories</legend>
        {TRACE_CATEGORIES.map((c) => (
          <label key={c.id} className="flex items-center gap-1" title={c.id}>
            <input
              type="checkbox"
              checked={selected?.has(c.id) ?? false}
              onChange={() => toggle(c.id)}
            />
            {c.label}
          </label>
        ))}
      </fieldset>
      <div className="flex items-center gap-2">
        {tracing ? (
          <Button size="sm" variant="destructive" disabled={busy} onClick={() => void stop()}>
            {busy ? "Collecting…" : "Stop"}
          </Button>
        ) : (
          <Button
            size="sm"
            variant="primary"
            disabled={busy || !selected || selected.size === 0}
            onClick={() => void start()}
          >
            Start recording
          </Button>
        )}
        {tracing && !busy ? (
          <span role="status" className="text-xs text-muted-foreground">
            Recording… use the app, then stop.
          </span>
        ) : null}
        {summary && !tracing ? (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => void save(backend.performance.saveTrace)}
          >
            Save trace…
          </Button>
        ) : null}
      </div>
      {summary && !tracing ? (
        <p className="text-xs text-muted-foreground">
          {formatDuration(summary.durationMs)} · {summary.events.toLocaleString()} events ·{" "}
          {formatBytes(summary.bytes)}
          {summary.dataLoss ? (
            <span className="text-warning"> · the app dropped some events</span>
          ) : null}
        </p>
      ) : null}
    </Card>
  );
}

function ProfileCard() {
  const backend = useBackend();
  const profiling = usePerformanceStore((s) => s.status.profiling);
  const { busy, error, saved, setSaved, run, save } = useAction();
  const [summary, setSummary] = useState<ProfileSummary | null>(null);

  const stop = async () => {
    const result = await run(backend.performance.stopProfile);
    if (result) {
      setSummary(result);
      setSaved(null);
    }
  };

  return (
    <Card
      title="CPU profile"
      description="Samples the JavaScript thread. Opens in Chrome DevTools or speedscope."
      error={error}
      saved={saved}
    >
      <div className="flex items-center gap-2">
        {profiling ? (
          <Button size="sm" variant="destructive" disabled={busy} onClick={() => void stop()}>
            Stop
          </Button>
        ) : (
          <Button
            size="sm"
            variant="primary"
            disabled={busy}
            onClick={() => void run(backend.performance.startProfile)}
          >
            Start profiling
          </Button>
        )}
        {profiling ? (
          <span role="status" className="text-xs text-muted-foreground">
            Profiling…
          </span>
        ) : null}
        {summary && !profiling ? (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => void save(backend.performance.saveProfile)}
          >
            Save profile…
          </Button>
        ) : null}
      </div>
      {summary && !profiling ? (
        <p className="text-xs text-muted-foreground">
          {formatDuration(summary.durationMs)} · {summary.samples.toLocaleString()} samples ·{" "}
          {summary.nodes.toLocaleString()} nodes · {formatBytes(summary.bytes)}
        </p>
      ) : null}
    </Card>
  );
}

function HeapCard() {
  const backend = useBackend();
  const snapshotting = usePerformanceStore((s) => s.status.snapshotting);
  const { busy, error, run } = useAction();
  const [result, setResult] = useState<HeapSnapshotResult | null>(null);

  const take = async () => {
    const r = await run(backend.performance.saveHeapSnapshot);
    if (r?.path) setResult(r);
  };

  return (
    <Card
      title="Heap snapshot"
      description="Saves the Hermes heap as a .heapsnapshot for the Chrome DevTools Memory panel."
      error={error}
      saved={result ? `${result.path} (${formatBytes(result.bytes)})` : null}
    >
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="primary"
          disabled={busy || snapshotting}
          onClick={() => void take()}
        >
          {busy || snapshotting ? "Taking snapshot…" : "Take snapshot…"}
        </Button>
      </div>
    </Card>
  );
}
