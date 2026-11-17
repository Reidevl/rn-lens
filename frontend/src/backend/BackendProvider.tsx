import type { ReactNode } from "react";

import type { Backend } from "./Backend";
import { BackendContext } from "./context";

export function BackendProvider({ backend, children }: { backend: Backend; children: ReactNode }) {
  return <BackendContext.Provider value={backend}>{children}</BackendContext.Provider>;
}
