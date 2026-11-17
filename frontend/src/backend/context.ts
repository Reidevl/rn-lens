import { createContext } from "react";

import type { Backend } from "./Backend";

export const BackendContext = createContext<Backend | null>(null);
