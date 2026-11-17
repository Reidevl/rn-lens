import type { SessionSnapshot, SessionState } from "@/backend";

export type Tone = "neutral" | "ok" | "warn" | "bad" | "info";

export interface SessionPresentation {
  /** Short label for the status bar and cards. */
  label: string;
  tone: Tone;
  /** Longer explanation for the attached card. */
  detail: string;
}

const byState: Record<SessionState, { label: string; tone: Tone }> = {
  idle: { label: "No app connected", tone: "neutral" },
  connecting: { label: "Connecting", tone: "info" },
  attached: { label: "Connected", tone: "ok" },
  reconnecting: { label: "Reconnecting", tone: "warn" },
  evicted: { label: "Disconnected by another debugger", tone: "bad" },
  closed: { label: "Disconnected", tone: "neutral" },
};

/** Maps a session snapshot to what the user should read. */
export function presentSession(session: SessionSnapshot): SessionPresentation {
  const base = byState[session.state] ?? byState.idle;
  let detail = session.reason ?? "";
  if (session.state === "evicted") {
    detail =
      "React Native DevTools (or another client) took the debugger slot. Attach again to take it back.";
  } else if (session.state === "reconnecting") {
    detail = `Attempt ${session.attempts}. ${session.reason ?? ""}`.trim();
  }
  return { label: base.label, tone: base.tone, detail };
}

/** One-line summary of the attached app for the status bar. */
export function describeApp(session: SessionSnapshot): string | null {
  const md = session.metadata;
  if (!md) return session.target?.title ?? null;
  const parts = [
    md.appDisplayName || md.appIdentifier,
    md.platform,
    md.reactNativeVersion ? `RN ${md.reactNativeVersion}` : "",
  ];
  return parts.filter(Boolean).join(" · ");
}
