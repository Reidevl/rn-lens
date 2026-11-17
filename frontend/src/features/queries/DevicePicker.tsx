import { Wifi, WifiOff } from "lucide-react";

import { useBackend, type SdkDevice } from "@/backend";
import { Button } from "@/shared/ui/atoms/Button";

import { runQueriesAction, useQueriesStore } from "./queriesStore";

/** Picks the connected app and toggles its online state. */
export function DevicePicker({
  devices,
  deviceKey,
}: {
  devices: SdkDevice[];
  deviceKey: string | null;
}) {
  const backend = useBackend();
  const selectDevice = useQueriesStore((s) => s.selectDevice);
  const busy = useQueriesStore((s) => s.busy);
  const current = devices.find((d) => d.key === deviceKey);
  if (!current) return null;

  return (
    <div className="flex items-center gap-2">
      {devices.length > 1 ? (
        <select
          aria-label="Connected app"
          value={current.key}
          onChange={(e) => selectDevice(e.target.value)}
          className="h-7 rounded-md border border-border bg-background px-2 text-sm"
        >
          {devices.map((d) => (
            <option key={d.key} value={d.key}>
              {d.app.name || d.app.id} · {d.device.name || d.device.id}
            </option>
          ))}
        </select>
      ) : (
        <span className="text-sm">
          {current.app.name || current.app.id}
          <span className="text-muted-foreground">
            {" "}
            · {current.device.name || current.device.id}
          </span>
        </span>
      )}
      <Button
        size="sm"
        aria-pressed={!current.online}
        aria-label={current.online ? "Simulate offline" : "Go back online"}
        title={current.online ? "Simulate offline (onlineManager.setOnline(false))" : "Back online"}
        disabled={busy}
        onClick={() =>
          void runQueriesAction(backend, () =>
            backend.queries.setOnline(current.key, !current.online),
          )
        }
      >
        {current.online ? (
          <Wifi aria-hidden="true" className="size-3.5" />
        ) : (
          <WifiOff aria-hidden="true" className="size-3.5 text-warning" />
        )}
        {current.online ? "Online" : "Offline"}
      </Button>
    </div>
  );
}
