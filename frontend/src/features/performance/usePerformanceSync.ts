import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { pullPerformance } from "./performanceStore";

/**
 * One pull now and one per performance:changed notice, serialized so a
 * notice during a pull schedules exactly one more. Mounted by the status
 * bar, which is always on screen. A failed pull keeps the last status.
 */
export function usePerformanceSync(): void {
  const backend = useBackend();

  useEffect(() => {
    let active = true;
    let inFlight = false;
    let again = false;

    const pull = () => {
      if (!active) return;
      if (inFlight) {
        again = true;
        return;
      }
      inFlight = true;
      pullPerformance(backend)
        .catch(() => {
          // The next notice retries; the indicator keeps the last status.
        })
        .finally(() => {
          inFlight = false;
          if (again) {
            again = false;
            pull();
          }
        });
    };

    pull();
    const offPerf = backend.events.on(TOPICS.performanceChanged, pull);
    const offSession = backend.events.on(TOPICS.sessionChanged, pull);
    return () => {
      active = false;
      offPerf();
      offSession();
    };
  }, [backend]);
}
