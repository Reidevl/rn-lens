import { useEffect, useState } from "react";

import { createLensClient, type LensClientOptions, type LensClientState } from "./client";

/**
 * Creates one Lens client for the component's lifetime, connects on mount,
 * disconnects on unmount and returns the connection state.
 */
export function useLens(options: LensClientOptions = {}): LensClientState {
  const [client] = useState(() => createLensClient(options));
  const [state, setState] = useState<LensClientState>(client.state);

  useEffect(() => {
    const unsubscribe = client.onStateChange(setState);
    client.connect();
    return () => {
      unsubscribe();
      client.disconnect();
    };
  }, [client]);

  return state;
}
