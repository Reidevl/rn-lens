import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { pullConsole } from "./consoleStore";

/** One initial pull, then one pull per console:changed notice. */
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
    const off = backend.events.on(TOPICS.consoleChanged, pull);
    return () => {
      active = false;
      off();
    };
  }, [backend]);
}
