import { useEffect } from "react";

import { TOPICS, useBackend } from "@/backend";

import { pullSession, pullTargets } from "./targetsStore";

/**
 * Keeps the targets store in sync with Go: one initial pull, then a pull per
 * change notification (notify-then-pull, ADR-0003).
 */
export function useTargetsSync(): void {
  const backend = useBackend();

  useEffect(() => {
    let active = true;
    const safe = (fn: () => Promise<void>) => () => {
      if (active) void fn().catch(() => {});
    };

    void pullTargets(backend).catch(() => {});
    const offTargets = backend.events.on(
      TOPICS.targetsChanged,
      safe(() => pullTargets(backend)),
    );
    const offSession = backend.events.on(
      TOPICS.sessionChanged,
      safe(() => pullSession(backend)),
    );

    return () => {
      active = false;
      offTargets();
      offSession();
    };
  }, [backend]);
}
