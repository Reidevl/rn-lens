import type { PanelDefinition } from "../shell/panels";

/** Shown for every panel until its milestone lands. */
export function PlaceholderPanel({ panel }: { panel: PanelDefinition }) {
  const Icon = panel.icon;
  return (
    <section aria-labelledby={`panel-${panel.id}-title`} className="flex h-full flex-col">
      <header className="flex h-12 items-center border-b border-border px-4">
        <h1 id={`panel-${panel.id}-title`} className="text-sm font-semibold">
          {panel.label}
        </h1>
      </header>
      <div className="flex flex-1 flex-col items-center justify-center gap-3 text-muted-foreground">
        <Icon aria-hidden="true" className="size-8" />
        <p className="text-sm">
          {panel.label} arrives in milestone <span className="font-mono">{panel.milestone}</span>.
        </p>
      </div>
    </section>
  );
}
