import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

import { pullStorage, useStorageStore } from "./storageStore";

/**
 * One initial pull, then one pull per storage:changed notice and per device change, serialized so
 * a notice during a pull schedules exactly one more. A failed pull shows in
 * the error banner until the next successful one.
 */
export function useStorageSync(): void {
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
      inFlight = pullStorage(backend)
        .then(() => {
          if (active && syncFailed) {
            syncFailed = false;
            useStorageStore.getState().setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!active) return;
          syncFailed = true;
          useStorageStore.getState().setError(errorMessage(err));
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
    const off = backend.events.on(TOPICS.storageChanged, pull);
    // Picking another device needs its stores now, not at the next notice.
    const unsubscribe = useStorageStore.subscribe((state, previous) => {
      if (state.deviceKey !== previous.deviceKey && state.deviceKey) pull();
    });
    return () => {
      active = false;
      off();
      unsubscribe();
    };
  }, [backend]);
}
