import { PlaceholderPanel } from "../panels/PlaceholderPanel";
import { LeftRail } from "./LeftRail";
import { PANELS } from "./panels";
import { useShellStore } from "./shellStore";
import { StatusBar } from "./StatusBar";

export function Shell() {
  const activePanel = useShellStore((s) => s.activePanel);
  const panel = PANELS.find((p) => p.id === activePanel) ?? PANELS[0];

  return (
    <div className="grid h-full grid-cols-[var(--rail-width)_1fr] grid-rows-[1fr_auto] bg-background">
      <aside className="row-span-1">
        <LeftRail />
      </aside>
      <main className="min-w-0 overflow-hidden">
        <PlaceholderPanel panel={panel} />
      </main>
      <div className="col-span-2">
        <StatusBar />
      </div>
    </div>
  );
}
