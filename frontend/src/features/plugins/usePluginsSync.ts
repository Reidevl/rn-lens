import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

import { pullPlugins, usePluginsStore } from "./pluginsStore";

/**
 * One pull now, one per plugins:changed notice and one per device change,
 * serialized so a notice during a pull schedules exactly one more.
 */
export function usePluginsSync(): void {
  const backend = useBackend();
  const deviceKey = usePluginsStore((s) => s.deviceKey);

  useEffect(() => {
    let active = true;
    let inFlight = false;
    let again = false;
    let failed = false;

    const pull = () => {
      if (!active) return;
      if (inFlight) {
        again = true;
        return;
      }
      inFlight = true;
      pullPlugins(backend)
        .then(() => {
          if (active && failed) {
            failed = false;
            usePluginsStore.getState().setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!active) return;
          failed = true;
          usePluginsStore.getState().setError(errorMessage(err));
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
    const off = backend.events.on(TOPICS.pluginsChanged, pull);
    return () => {
      active = false;
      off();
    };
  }, [backend, deviceKey]);
}
