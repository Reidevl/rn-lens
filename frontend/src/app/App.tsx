import { BackendProvider, type Backend } from "@/backend";

import { Shell } from "./shell/Shell";
import { ThemeProvider } from "./theme/ThemeProvider";

export function App({ backend }: { backend: Backend }) {
  return (
    <BackendProvider backend={backend}>
      <ThemeProvider>
        <Shell />
      </ThemeProvider>
    </BackendProvider>
  );
}
