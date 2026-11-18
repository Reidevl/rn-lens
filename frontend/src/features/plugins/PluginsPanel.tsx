import { useBackend } from "@/backend";
import { cn } from "@/shared/lib/cn";

import { Block } from "./Blocks";
import { runPluginAction, usePluginsStore } from "./pluginsStore";
import { usePluginsSync } from "./usePluginsSync";

/** The panels apps add to Lens with `@rnlens/sdk` (createLensPanel). */
export function PluginsPanel() {
  const backend = useBackend();
  usePluginsSync();
  const devices = usePluginsStore((s) => s.devices);
  const deviceKey = usePluginsStore((s) => s.deviceKey);
  const plugins = usePluginsStore((s) => s.plugins);
  const pluginId = usePluginsStore((s) => s.pluginId);
  const running = usePluginsStore((s) => s.running);
  const error = usePluginsStore((s) => s.error);
  const notice = usePluginsStore((s) => s.notice);
  const selectDevice = usePluginsStore((s) => s.selectDevice);
  const selectPlugin = usePluginsStore((s) => s.selectPlugin);
  const plugin = plugins.find((p) => p.id === pluginId);

  if (devices.length === 0) return <NoPlugins />;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <header className="flex items-center gap-3 border-b border-border px-3 py-2">
        {devices.length > 1 ? (
          <select
            aria-label="Device"
            value={deviceKey ?? ""}
            onChange={(e) => selectDevice(e.target.value)}
            className="h-7 rounded-md border border-border bg-background px-1 text-xs"
          >
            {devices.map((d) => (
              <option key={d.key} value={d.key}>
                {d.app.name ?? d.app.id} · {d.device.name ?? d.device.id}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-xs text-muted-foreground">
            {devices[0].app.name ?? devices[0].app.id} · {devices[0].app.platform}
          </span>
        )}
        <div
          role="tablist"
          aria-label="Plugins"
          className="flex gap-1 overflow-x-auto"
          onKeyDown={(e) => {
            const i = plugins.findIndex((p) => p.id === pluginId);
            const next =
              e.key === "ArrowRight"
                ? (i + 1) % plugins.length
                : e.key === "ArrowLeft"
                  ? (i - 1 + plugins.length) % plugins.length
                  : -1;
            if (next < 0 || plugins.length === 0) return;
            e.preventDefault();
            selectPlugin(plugins[next].id);
            document.getElementById(`plugin-tab-${plugins[next].id}`)?.focus();
          }}
        >
          {plugins.map((p) => (
            <button
              key={p.id}
              id={`plugin-tab-${p.id}`}
              type="button"
              role="tab"
              aria-selected={p.id === pluginId}
              aria-controls="plugin-panel"
              tabIndex={p.id === pluginId ? 0 : -1}
              onClick={() => selectPlugin(p.id)}
              className={cn(
                "h-7 cursor-pointer rounded-md px-2 text-sm whitespace-nowrap transition-colors duration-150",
                p.id === pluginId
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:bg-muted/60",
              )}
            >
              {p.name}
            </button>
          ))}
        </div>
      </header>
      {error ? (
        <p
          role="alert"
          className="border-b border-border bg-destructive/10 px-3 py-1 text-xs text-destructive"
        >
          {error}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="border-b border-border bg-muted/40 px-3 py-1 text-xs">
          {notice}
        </p>
      ) : null}
      {plugins.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">
          This app has not registered a panel yet.
        </p>
      ) : plugin ? (
        <div
          id="plugin-panel"
          role="tabpanel"
          aria-labelledby={`plugin-tab-${plugin.id}`}
          className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-3"
        >
          {plugin.description ? (
            <p className="text-xs text-muted-foreground">{plugin.description}</p>
          ) : null}
          {plugin.error ? (
            <p role="alert" className="text-sm text-destructive">
              {plugin.error}
            </p>
          ) : null}
          {plugin.blocks.length === 0 && !plugin.error ? (
            <p className="text-sm text-muted-foreground">This panel is empty.</p>
          ) : null}
          {plugin.blocks.map((block, i) => (
            <Block
              key={`${block.id}-${i}`}
              block={block}
              running={running}
              onAction={(blockId, actionId, rowId) =>
                void runPluginAction(backend, {
                  pluginId: plugin.id,
                  blockId,
                  actionId,
                  ...(rowId ? { rowId } : {}),
                })
              }
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

const SNIPPET = `import { createLensPanel, useLens } from "@rnlens/sdk";

const panel = createLensPanel({
  id: "my-panel",
  name: "My panel",
  render: () => [{ type: "text", id: "hello", text: "Hello from the app" }],
});

useLens({ panels: [panel] });`;

function NoPlugins() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-sm text-muted-foreground">
      <p className="font-medium text-foreground">No app has added a panel to Lens yet.</p>
      <p>Describe one with building blocks; Lens draws it:</p>
      <pre className="rounded-md border border-border bg-card p-3 font-mono text-xs text-foreground">
        {SNIPPET}
      </pre>
    </div>
  );
}
