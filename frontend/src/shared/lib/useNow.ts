import { useEffect, useState } from "react";

/**
 * Current time in milliseconds, refreshed every `intervalMs` while mounted.
 * Lets relative labels such as "5s ago" keep moving between data pulls.
 */
export function useNow(intervalMs = 1_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
