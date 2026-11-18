import type { FastRefreshState } from "@/backend";
import type { Tone } from "@/features/targets/sessionPresentation";

/** The trace categories Lens offers, in display order. */
export const TRACE_CATEGORIES: readonly { id: string; label: string }[] = [
  { id: "disabled-by-default-v8.cpu_profiler", label: "JavaScript sampling" },
  { id: "v8.execute", label: "JavaScript execution" },
  { id: "devtools.timeline", label: "Timeline" },
  { id: "disabled-by-default-devtools.timeline", label: "Timeline (detailed)" },
  { id: "disabled-by-default-devtools.timeline.frame", label: "Frames" },
  { id: "blink.user_timing", label: "User timing" },
  { id: "disabled-by-default-devtools.screenshot", label: "Screenshots" },
];

/** "850 ms", "4.2 s", "1 min 5 s". */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)} min ${s % 60} s`;
}

/** The status bar text and dot for the last Fast Refresh. */
export function presentFastRefresh(
  state: FastRefreshState,
  now: number,
): { label: string; tone: Tone; title?: string } | null {
  switch (state.phase) {
    case "off":
      return null;
    case "unavailable":
      return { label: "Fast Refresh unavailable", tone: "neutral", title: state.message };
    case "connecting":
      return { label: "Fast Refresh connecting…", tone: "neutral" };
    case "ready":
      return { label: "Fast Refresh ready", tone: "ok" };
    case "updating":
      return { label: "Fast Refresh updating…", tone: "info" };
    case "updated": {
      const n = state.modules ?? 0;
      const ago = state.at ? since(Date.parse(state.at), now) : "";
      return {
        label: `Fast Refresh: ${n} ${n === 1 ? "module" : "modules"}${ago ? ` ${ago}` : ""}`,
        tone: "ok",
      };
    }
    case "error":
      return { label: "Fast Refresh failed", tone: "bad", title: state.message };
    default:
      // A phase this UI does not know yet: show nothing rather than guess.
      return null;
  }
}

function since(at: number, now: number): string {
  if (Number.isNaN(at)) return "";
  const s = Math.max(0, Math.round((now - at) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return `${s} s ago`;
  return `${Math.floor(s / 60)} min ago`;
}
