import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";
import { errorMessage } from "@/shared/lib/errorMessage";

import { pullMcp, useMcpStore } from "./mcpStore";

/** One pull now and one per mcp:changed notice, serialized. */
export function useMcpSync(): void {
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
      pullMcp(backend)
        .then(() => {
          if (active) useMcpStore.getState().setError(null);
        })
        .catch((err: unknown) => {
          if (active) useMcpStore.getState().setError(errorMessage(err));
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
    const off = backend.events.on(TOPICS.mcpChanged, pull);
    return () => {
      active = false;
      off();
    };
  }, [backend]);
}
