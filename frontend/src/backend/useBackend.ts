import { useContext } from "react";

import type { Backend } from "./Backend";
import { BackendContext } from "./context";

/** Returns the active Backend. Throws when rendered outside a BackendProvider. */
export function useBackend(): Backend {
  const backend = useContext(BackendContext);
  if (!backend) {
    throw new Error("useBackend must be used inside <BackendProvider>");
  }
  return backend;
}
