import { X } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";

import { useBackend, type NetworkBody, type NetworkRequest, type NetworkSummary } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";
import { CopyButton } from "@/shared/ui/molecules/CopyButton";
import { JsonTree } from "@/shared/ui/molecules/JsonTree";

import { useNetworkStore } from "./networkStore";
import {
  formatBytes,
  formatMillis,
  isImageMime,
  isJSONMime,
  queryParams,
  statusLabel,
  toCurl,
} from "./presentation";

const TABS = ["Headers", "Payload", "Preview", "Response", "Timing", "Initiator"] as const;
type Tab = (typeof TABS)[number];

type Loaded<T> =
  | { state: "idle" | "loading" }
  | { state: "ready"; value: T }
  | { state: "error"; message: string };

/**
 * Detail of one request. The full record is re-read whenever the row's
 * version changes; bodies are fetched only when a tab needs them.
 */
export function RequestDetail({ row, onClose }: { row: NetworkSummary; onClose: () => void }) {
  const backend = useBackend();
  const [tab, setTab] = useState<Tab>("Headers");
  const [loaded, setLoaded] = useState<{ seq: number; request?: NetworkRequest; error?: string }>({
    seq: -1,
  });
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);
  // Bodies keyed by "seq:epoch" so switching rows never shows another row's body.
  const [responses, setResponses] = useState<Record<string, Loaded<NetworkBody>>>({});
  const [payloads, setPayloads] = useState<Record<string, Loaded<string>>>({});
  const requested = useRef(new Set<string>());
  const key = `${row.seq}:${row.epoch}`;

  useEffect(() => {
    let active = true;
    backend.network.request(row.seq).then(
      (request) => active && setLoaded({ seq: row.seq, request }),
      (err: unknown) => active && setLoaded({ seq: row.seq, error: errorMessage(err) }),
    );
    return () => {
      active = false;
    };
  }, [backend, row.seq, row.version]);
  const request = loaded.seq === row.seq ? (loaded.request ?? null) : null;
  const loadError = loaded.seq === row.seq ? loaded.error : undefined;

  const wantsResponse = (tab === "Response" || tab === "Preview") && row.state === "complete";
  useEffect(() => {
    const id = `response:${key}`;
    if (!wantsResponse || requested.current.has(id)) return;
    requested.current.add(id);
    backend.network.responseBody(row.seq, row.epoch).then(
      (value) => setResponses((m) => ({ ...m, [key]: { state: "ready", value } })),
      (err: unknown) =>
        setResponses((m) => ({ ...m, [key]: { state: "error", message: errorMessage(err) } })),
    );
  }, [backend, wantsResponse, key, row.seq, row.epoch]);

  const hasBody = request?.hasPostData ?? false;
  useEffect(() => {
    const id = `payload:${key}`;
    if (!hasBody || requested.current.has(id)) return;
    requested.current.add(id);
    backend.network.requestBody(row.seq, row.epoch).then(
      (value) => setPayloads((m) => ({ ...m, [key]: { state: "ready", value } })),
      (err: unknown) =>
        setPayloads((m) => ({ ...m, [key]: { state: "error", message: errorMessage(err) } })),
    );
  }, [backend, hasBody, key, row.seq, row.epoch]);

  const response: Loaded<NetworkBody> = responses[key] ?? { state: "loading" };
  const payload: Loaded<string> = payloads[key] ?? { state: "loading" };
  // The Payload tab may not have loaded the body yet; cURL needs it.
  const curl = async (r: NetworkRequest) => {
    let body = payload.state === "ready" ? payload.value : "";
    let missing: string | null = null;
    if (!body && r.hasPostData) {
      body = await backend.network.requestBody(r.seq, r.epoch).catch((err: unknown) => {
        missing = errorMessage(err);
        return "";
      });
    }
    await backend.clipboard.writeText(toCurl(r, body));
    if (missing) {
      useNetworkStore
        .getState()
        .setNotice(`Copied the cURL command without the request body: ${missing}`);
    }
  };

  // Uses the body a tab already loaded, or fetches it once.
  const copyResponse = async () => {
    const cached = responses[key];
    let body: NetworkBody;
    if (cached?.state === "ready") {
      body = cached.value;
    } else {
      // The Response and Preview tabs reuse this fetch instead of repeating it.
      // A failure is recorded too, so those tabs show it instead of waiting.
      requested.current.add(`response:${key}`);
      try {
        body = await backend.network.responseBody(row.seq, row.epoch);
      } catch (err) {
        setResponses((m) => ({ ...m, [key]: { state: "error", message: errorMessage(err) } }));
        throw err;
      }
      const value = body;
      setResponses((m) => ({ ...m, [key]: { state: "ready", value } }));
    }
    if (body.truncated) throw new Error("The body is larger than Lens shows");
    await backend.clipboard.writeText(body.text);
  };

  const onTabKey = (e: React.KeyboardEvent, index: number) => {
    const last = TABS.length - 1;
    const next =
      e.key === "ArrowRight"
        ? (index + 1) % TABS.length
        : e.key === "ArrowLeft"
          ? (index - 1 + TABS.length) % TABS.length
          : e.key === "Home"
            ? 0
            : e.key === "End"
              ? last
              : -1;
    if (next < 0) return;
    e.preventDefault();
    setTab(TABS[next]);
    tabRefs.current[next]?.focus();
  };

  return (
    <section aria-label="Request details" className="flex min-h-0 flex-col border-l border-border">
      <div className="flex items-center gap-1 border-b border-border px-2 py-1">
        <div
          role="tablist"
          aria-label="Request detail tabs"
          className="flex flex-1 flex-wrap gap-1"
        >
          {TABS.map((t, i) => (
            <button
              key={t}
              ref={(el) => {
                tabRefs.current[i] = el;
              }}
              id={`network-tab-${t}`}
              type="button"
              role="tab"
              aria-selected={tab === t}
              aria-controls="network-tabpanel"
              tabIndex={tab === t ? 0 : -1}
              onKeyDown={(e) => onTabKey(e, i)}
              onClick={() => setTab(t)}
              className={cn(
                "h-7 cursor-pointer rounded-md px-2 text-xs transition-colors duration-150",
                tab === t ? "bg-muted text-foreground" : "text-muted-foreground hover:bg-muted/60",
              )}
            >
              {t}
            </button>
          ))}
        </div>
        {row.state === "complete" ? (
          <CopyButton label="Copy response" text="Copy response" copy={copyResponse} />
        ) : null}
        {request ? (
          <CopyButton label="Copy as cURL" text="Copy as cURL" copy={() => curl(request)} />
        ) : null}
        <Button size="icon" aria-label="Close details" onClick={onClose}>
          <X aria-hidden="true" className="size-4" />
        </Button>
      </div>
      <div
        id="network-tabpanel"
        role="tabpanel"
        aria-labelledby={`network-tab-${tab}`}
        className="min-h-0 flex-1 overflow-auto p-3 text-xs"
      >
        {loadError ? (
          <p role="alert" className="text-destructive">
            {loadError}
          </p>
        ) : !request ? (
          <p className="text-muted-foreground">Loading…</p>
        ) : tab === "Headers" ? (
          <HeadersTab request={request} />
        ) : tab === "Payload" ? (
          <PayloadTab request={request} body={payload} />
        ) : tab === "Preview" ? (
          <BodyTab request={request} body={response} preview />
        ) : tab === "Response" ? (
          <BodyTab request={request} body={response} />
        ) : tab === "Timing" ? (
          <TimingTab request={request} />
        ) : (
          <InitiatorTab request={request} />
        )}
      </div>
    </section>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mb-4">
      <h2 className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Pairs({ pairs }: { pairs: Array<[string, string]> }) {
  if (pairs.length === 0) return <p className="text-muted-foreground">None</p>;
  return (
    <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-3 gap-y-0.5 font-mono">
      {pairs.map(([k, v], i) => (
        <div key={`${k}-${i}`} className="contents">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="break-all select-text">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

const sortedHeaders = (h: Record<string, string> | null): Array<[string, string]> =>
  Object.entries(h ?? {}).sort(([a], [b]) => a.localeCompare(b));

function HeadersTab({ request: r }: { request: NetworkRequest }) {
  const general: Array<[string, string]> = [
    ["Request URL", r.url],
    ["Request Method", r.method],
    ["Status Code", `${statusLabel(r)} ${r.statusText}`.trim()],
  ];
  if (r.remoteAddress) general.push(["Remote Address", r.remoteAddress]);
  if (r.protocol) general.push(["Protocol", r.protocol]);
  if (r.errorText) general.push(["Error", r.errorText]);
  return (
    <>
      <Section title="General">
        <Pairs pairs={general} />
      </Section>
      <Section title="Response headers">
        <Pairs pairs={sortedHeaders(r.responseHeaders)} />
      </Section>
      <Section title="Request headers">
        <Pairs pairs={sortedHeaders(r.requestHeaders)} />
      </Section>
    </>
  );
}

function parseJSON(text: string): { ok: true; value: unknown } | { ok: false } {
  try {
    return { ok: true, value: JSON.parse(text) as unknown };
  } catch {
    return { ok: false };
  }
}

function PayloadTab({ request: r, body }: { request: NetworkRequest; body: Loaded<string> }) {
  const params = queryParams(r.url);
  return (
    <>
      {params.length > 0 ? (
        <Section title="Query string parameters">
          <Pairs pairs={params} />
        </Section>
      ) : null}
      <Section title="Request body">
        {!r.hasPostData ? (
          <p className="text-muted-foreground">This request has no body.</p>
        ) : body.state === "ready" ? (
          <Text text={body.value} />
        ) : body.state === "error" ? (
          <p role="alert" className="text-destructive">
            {body.message}
          </p>
        ) : (
          <p className="text-muted-foreground">Loading…</p>
        )}
      </Section>
    </>
  );
}

/** JSON as a tree when it parses, otherwise the raw text. */
function Text({ text }: { text: string }) {
  const parsed = parseJSON(text);
  return parsed.ok && typeof parsed.value === "object" && parsed.value !== null ? (
    <JsonTree value={parsed.value} label="Body" />
  ) : (
    <pre className="font-mono break-all whitespace-pre-wrap select-text">{text}</pre>
  );
}

function BodyTab({
  request: r,
  body,
  preview = false,
}: {
  request: NetworkRequest;
  body: Loaded<NetworkBody>;
  preview?: boolean;
}) {
  if (r.state === "pending")
    return <p className="text-muted-foreground">The response has not finished yet.</p>;
  if (r.state === "failed")
    return <p className="text-muted-foreground">The request failed; there is no response body.</p>;
  if (body.state === "error")
    return (
      <p role="alert" className="text-destructive">
        {body.message}
      </p>
    );
  if (body.state !== "ready") return <p className="text-muted-foreground">Loading…</p>;
  const b = body.value;
  if (b.truncated)
    return (
      <p className="text-muted-foreground">
        The body is {formatBytes(b.size)}, larger than Lens shows.
      </p>
    );
  if (b.text === "") return <p className="text-muted-foreground">The response has no body.</p>;
  if (preview && b.base64 && isImageMime(r.mimeType)) {
    return (
      <img
        alt={`Response image (${r.mimeType})`}
        src={`data:${r.mimeType};base64,${b.text}`}
        className="max-w-full"
      />
    );
  }
  if (b.base64)
    return <p className="text-muted-foreground">Binary body ({formatBytes(b.size)}, base64).</p>;
  if (preview && (isJSONMime(r.mimeType) || parseJSON(b.text).ok)) {
    const parsed = parseJSON(b.text);
    if (parsed.ok) return <JsonTree value={parsed.value} label="Response preview" />;
  }
  return <pre className="font-mono break-all whitespace-pre-wrap select-text">{b.text}</pre>;
}

function TimingTab({ request: r }: { request: NetworkRequest }) {
  const t = r.timing;
  const phases: Array<[string, number]> = [];
  if (t) {
    const span = (from: number, to: number) => (from >= 0 && to >= from ? to - from : -1);
    phases.push([
      "Queueing",
      t.dnsStart >= 0 ? t.dnsStart : t.connectStart >= 0 ? t.connectStart : t.sendStart,
    ]);
    phases.push(["DNS lookup", span(t.dnsStart, t.dnsEnd)]);
    phases.push(["Initial connection", span(t.connectStart, t.connectEnd)]);
    phases.push(["SSL", span(t.sslStart, t.sslEnd)]);
    phases.push(["Request sent", span(t.sendStart, t.sendEnd)]);
    phases.push(["Waiting for server", span(t.sendEnd, t.receiveHeadersEnd)]);
    if (r.endTime > 0)
      phases.push(["Content download", (r.endTime - t.requestTime) * 1000 - t.receiveHeadersEnd]);
  } else {
    if (r.responseTime > 0)
      phases.push(["Waiting for server", (r.responseTime - r.startTime) * 1000]);
    if (r.endTime > 0 && r.responseTime > 0)
      phases.push(["Content download", (r.endTime - r.responseTime) * 1000]);
  }
  const shown = phases.filter(([, ms]) => ms >= 0);
  const total = r.endTime > 0 ? (r.endTime - r.startTime) * 1000 : 0;
  const max = Math.max(total, ...shown.map(([, ms]) => ms), 1);
  if (shown.length === 0)
    return <p className="text-muted-foreground">The app reported no timing for this request.</p>;
  return (
    <>
      <ul className="flex flex-col gap-1">
        {shown.map(([name, ms]) => (
          <li key={name} className="grid grid-cols-[10rem_1fr_5rem] items-center gap-2">
            <span>{name}</span>
            <span className="h-2 rounded-sm bg-muted">
              <span
                className="block h-2 rounded-sm bg-accent/70"
                style={{ width: `${Math.max((ms / max) * 100, 0.5)}%` }}
              />
            </span>
            <span className="text-right tabular-nums">{formatMillis(ms)}</span>
          </li>
        ))}
      </ul>
      {total > 0 ? <p className="mt-3 text-muted-foreground">Total {formatMillis(total)}</p> : null}
    </>
  );
}

function InitiatorTab({ request: r }: { request: NetworkRequest }) {
  const i = r.initiator;
  if (!i) return <p className="text-muted-foreground">The app reported no initiator.</p>;
  const pairs: Array<[string, string]> = [["Type", i.type]];
  if (i.url) pairs.push(["URL", i.lineNumber != null ? `${i.url}:${i.lineNumber + 1}` : i.url]);
  return (
    <>
      <Section title="Initiator">
        <Pairs pairs={pairs} />
      </Section>
      {i.stack ? (
        <Section title="Stack">
          <JsonTree value={i.stack} label="Initiator stack" />
        </Section>
      ) : null}
    </>
  );
}
