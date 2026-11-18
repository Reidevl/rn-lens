import { ConsolePanel } from "@/features/console/ConsolePanel";
import { NetworkPanel } from "@/features/network/NetworkPanel";
import { MutationsPanel } from "@/features/queries/MutationsPanel";
import { QueriesPanel } from "@/features/queries/QueriesPanel";
import { StoragePanel } from "@/features/storage/StoragePanel";
import { TargetsPanel } from "@/features/targets/TargetsPanel";
import { useTargetsSync } from "@/features/targets/useTargetsSync";

import { PlaceholderPanel } from "../panels/PlaceholderPanel";
import { LeftRail } from "./LeftRail";
import { PanelErrorBoundary } from "./PanelErrorBoundary";
import { PANELS } from "./panels";
import { useShellStore } from "./shellStore";
import { StatusBar } from "./StatusBar";

export function Shell() {
  const activePanel = useShellStore((s) => s.activePanel);
  const panel = PANELS.find((p) => p.id === activePanel) ?? PANELS[0];
  useTargetsSync();

  return (
    <div className="grid h-full grid-cols-[var(--rail-width)_1fr] grid-rows-[1fr_auto] bg-background">
      <aside className="row-span-1">
        <LeftRail />
      </aside>
      <main className="min-w-0 overflow-hidden">
        <PanelErrorBoundary key={panel.id} panel={panel.label}>
          {panel.id === "targets" ? (
            <TargetsPanel />
          ) : panel.id === "console" ? (
            <ConsolePanel />
          ) : panel.id === "network" ? (
            <NetworkPanel />
          ) : panel.id === "storage" ? (
            <StoragePanel />
          ) : panel.id === "queries" ? (
            <QueriesPanel />
          ) : panel.id === "mutations" ? (
            <MutationsPanel />
          ) : (
            <PlaceholderPanel panel={panel} />
          )}
        </PanelErrorBoundary>
      </main>
      <div className="col-span-2">
        <StatusBar />
      </div>
    </div>
  );
}
