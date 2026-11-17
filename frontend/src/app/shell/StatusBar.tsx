import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";

import { useBackend, type AppInfo } from "@/backend";
import { Button } from "@/shared/ui/atoms/Button";

import { useTheme } from "../theme/useTheme";

export function StatusBar() {
  const backend = useBackend();
  const { resolved, toggle } = useTheme();
  const [info, setInfo] = useState<AppInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    backend.app
      .info()
      .then((value) => {
        if (!cancelled) setInfo(value);
      })
      .catch(() => {
        // The status bar degrades to "Lens" without a version.
      });
    return () => {
      cancelled = true;
    };
  }, [backend]);

  return (
    <footer
      role="status"
      className="flex h-(--statusbar-height) items-center justify-between border-t border-border bg-card px-3 text-xs text-muted-foreground"
    >
      <div className="flex items-center gap-3">
        <span className="flex items-center gap-1.5">
          <span aria-hidden="true" className="size-2 rounded-full bg-muted-foreground" />
          No app connected
        </span>
      </div>
      <div className="flex items-center gap-2">
        <span className="font-mono">{info ? `${info.name} v${info.version}` : "Lens"}</span>
        <Button
          size="icon"
          className="h-6 w-6"
          aria-label={resolved === "dark" ? "Switch to light theme" : "Switch to dark theme"}
          onClick={toggle}
        >
          {resolved === "dark" ? (
            <Sun aria-hidden="true" className="size-3.5" />
          ) : (
            <Moon aria-hidden="true" className="size-3.5" />
          )}
        </Button>
      </div>
    </footer>
  );
}
