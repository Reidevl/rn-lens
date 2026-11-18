import type { CSSProperties } from "react";

import { cn } from "@/shared/lib/cn";

import { PANELS } from "./panels";
import { useShellStore } from "./shellStore";
import { hasInsetTitleBar } from "./titleBar";

export function LeftRail() {
  const activePanel = useShellStore((s) => s.activePanel);
  const setActivePanel = useShellStore((s) => s.setActivePanel);
  const inset = hasInsetTitleBar();

  return (
    <nav aria-label="Panels" className="flex h-full flex-col border-r border-border bg-card">
      <div
        data-titlebar={inset ? "inset" : undefined}
        // In the macOS app this row is the title bar: it drags the window and
        // leaves room for the close, minimize and zoom buttons on its left.
        style={inset ? ({ "--wails-draggable": "drag" } as CSSProperties) : undefined}
        className={cn("flex h-12 shrink-0 items-center pt-1", inset ? "pr-4 pl-20" : "px-4")}
      >
        <span className="font-mono text-sm font-semibold tracking-wide text-foreground">Lens</span>
      </div>
      <ul className="flex flex-col gap-0.5 px-2">
        {PANELS.map(({ id, label, icon: Icon }) => {
          const active = id === activePanel;
          return (
            <li key={id}>
              <button
                type="button"
                aria-current={active ? "page" : undefined}
                onClick={() => setActivePanel(id)}
                className={cn(
                  "flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-sm transition-colors duration-150",
                  active
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                )}
              >
                <Icon aria-hidden="true" className="size-4 shrink-0" />
                <span>{label}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
