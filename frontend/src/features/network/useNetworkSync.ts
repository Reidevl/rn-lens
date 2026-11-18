import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

import { pullNetwork, useNetworkStore } from "./networkStore";

/**
 * One initial pull, then one pull per network:changed notice, serialized so
 * a notice during a pull schedules exactly one more. A failed pull shows in
 * the error banner until the next successful one.
 */
export function useNetworkSync(): void {
  const backend = useBackend();

  useEffect(() => {
    let active = true;
    let inFlight: Promise<void> | null = null;
    let again = false;
    let syncFailed = false;

    const pull = () => {
      if (!active) return;
      if (inFlight) {
        again = true;
        return;
      }
      inFlight = pullNetwork(backend)
        .then(() => {
          if (active && syncFailed) {
            syncFailed = false;
            useNetworkStore.getState().setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!active) return;
          syncFailed = true;
          useNetworkStore.getState().setError(errorMessage(err));
        })
        .finally(() => {
          inFlight = null;
          if (again) {
            again = false;
            pull();
          }
        });
    };

    pull();
    const off = backend.events.on(TOPICS.networkChanged, pull);
    return () => {
      active = false;
      off();
    };
  }, [backend]);
}
