import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { pullQueries } from "./queriesStore";

/** One initial pull, then one serialized pull per queries:changed notice. */
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
    return () => {
      active = false;
      off();
    };
  }, [backend]);
}
