import { cn } from "@/shared/lib/cn";

import { PANELS } from "./panels";
import { useShellStore } from "./shellStore";

export function LeftRail() {
  const activePanel = useShellStore((s) => s.activePanel);
  const setActivePanel = useShellStore((s) => s.setActivePanel);

  return (
    <nav aria-label="Panels" className="flex h-full flex-col border-r border-border bg-card">
      <div className="flex h-12 items-center px-4 pt-1">
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
