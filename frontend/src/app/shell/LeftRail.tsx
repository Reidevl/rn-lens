import type { CSSProperties } from "react";

import { cn } from "@/shared/lib/cn";

import { PANELS, type PanelDefinition } from "./panels";
import { useShellStore } from "./shellStore";
import { hasInsetTitleBar } from "./titleBar";

export function LeftRail() {
  const inset = hasInsetTitleBar();

  return (
    <nav aria-label="Panels" className="flex h-full flex-col border-r border-border bg-card">
      {inset ? (
        // macOS: like Finder or Mail, the top of the sidebar belongs to the
        // window buttons. It stays empty and drags the window; the panels
        // start below it.
        <div
          data-titlebar="inset"
          aria-hidden="true"
          style={{ "--wails-draggable": "drag" } as CSSProperties}
          className="h-[52px] shrink-0"
        />
      ) : (
        <div className="flex h-12 shrink-0 items-center px-4 pt-1">
          <span className="font-mono text-sm font-semibold tracking-wide text-foreground">
            Lens
          </span>
        </div>
      )}
      <PanelList panels={PANELS.filter((p) => !p.placement)} />
      <div className="mt-auto border-t border-border py-2">
        <PanelList panels={PANELS.filter((p) => p.placement === "bottom")} />
      </div>
    </nav>
  );
}

function PanelList({ panels }: { panels: readonly PanelDefinition[] }) {
  const activePanel = useShellStore((s) => s.activePanel);
  const setActivePanel = useShellStore((s) => s.setActivePanel);
  return (
    <ul className="flex flex-col gap-0.5 px-2">
      {panels.map(({ id, label, icon: Icon }) => {
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
  );
}
