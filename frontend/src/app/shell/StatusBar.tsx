import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";

import { useBackend, type AppInfo } from "@/backend";
import { McpStatus } from "@/features/mcp/McpStatus";
import { FastRefreshIndicator } from "@/features/performance/FastRefreshIndicator";
import { describeApp, presentSession, type Tone } from "@/features/targets/sessionPresentation";
import { useTargetsStore } from "@/features/targets/targetsStore";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/atoms/Button";

import { useTheme } from "../theme/useTheme";

const toneDot: Record<Tone, string> = {
  neutral: "bg-muted-foreground",
  ok: "bg-accent",
  warn: "bg-warning",
  bad: "bg-destructive",
  info: "bg-info",
};

export function StatusBar() {
  const backend = useBackend();
  const { resolved, toggle } = useTheme();
  const [info, setInfo] = useState<AppInfo | null>(null);
  const session = useTargetsStore((s) => s.session);
  const view = presentSession(session);
  const app = describeApp(session);

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
          <span aria-hidden="true" className={cn("size-2 rounded-full", toneDot[view.tone])} />
          {session.state === "attached" && app ? app : view.label}
        </span>
        <FastRefreshIndicator />
      </div>
      <div className="flex items-center gap-2">
        <McpStatus />
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
