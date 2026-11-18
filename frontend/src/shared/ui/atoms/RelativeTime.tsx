import { relativeTime } from "@/shared/lib/relativeTime";
import { useNow } from "@/shared/lib/useNow";

/**
 * "5s ago" that keeps ticking. Owning the clock here confines the per-second
 * re-render to this span instead of the whole panel around it.
 */
export function RelativeTime({ ms, className }: { ms: number; className?: string }) {
  const now = useNow();
  return <span className={className}>{relativeTime(ms, now)}</span>;
}
