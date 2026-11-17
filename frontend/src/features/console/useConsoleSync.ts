import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { errorMessage, pullConsole, useConsoleStore } from "./consoleStore";

/**
 * One initial pull, then one pull per console:changed notice. A failed pull
 * lands in the store's error banner so a broken sync is never silent.
 */
export function useConsoleSync(): void {
  const backend = useBackend();

  useEffect(() => {
    let active = true;
    let inFlight: Promise<void> | null = null;
    let again = false;

    // Serialize pulls: a notice during a pull schedules exactly one more.
    const pull = () => {
      if (!active) return;
      if (inFlight) {
        again = true;
        return;
      }
      inFlight = pullConsole(backend)
        .catch((err: unknown) => {
          if (active) useConsoleStore.getState().setError(errorMessage(err));
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
    const off = backend.events.on(TOPICS.consoleChanged, pull);
    return () => {
      active = false;
      off();
    };
  }, [backend]);
}
