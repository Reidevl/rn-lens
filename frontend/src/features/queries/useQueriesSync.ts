import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { pullQueries, useQueriesStore } from "./queriesStore";

/**
 * One initial pull, then one serialized pull per queries:changed notice and
 * per device selection, so switching device never waits for the next notice.
 */
export function useQueriesSync(): void {
  const backend = useBackend();

  useEffect(() => {
    let active = true;
    let inFlight: Promise<void> | null = null;
    let again = false;

    const pull = () => {
      if (!active) return;
      if (inFlight) {
        again = true;
        return;
      }
      inFlight = pullQueries(backend)
        .catch(() => {})
        .finally(() => {
          inFlight = null;
          if (again) {
            again = false;
            pull();
          }
        });
    };

    pull();
    const off = backend.events.on(TOPICS.queriesChanged, pull);
    const unsubscribe = useQueriesStore.subscribe((state, previous) => {
      if (state.deviceKey !== previous.deviceKey && state.deviceKey) pull();
    });
    return () => {
      active = false;
      off();
      unsubscribe();
    };
  }, [backend]);
}
