import { Events } from "@wailsio/runtime";

import * as AppService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/appservice";
import * as ConsoleService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/consoleservice";
import * as TargetsService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/targetsservice";
import type { Backend, ConsoleEntry, ConsoleProperty, SessionSnapshot } from "./Backend";

/** Backend backed by the Wails v3 bindings and event bus. */
export function createWailsBackend(): Backend {
  return {
    kind: "wails",
    app: {
      info: () => AppService.Info(),
    },
    targets: {
      list: async () => (await TargetsService.List()) ?? [],
      refresh: () => TargetsService.Refresh(),
      // The generated enum is structurally a string union; narrow it once here.
      session: async () => (await TargetsService.Session()) as unknown as SessionSnapshot,
      attach: (id) => TargetsService.Attach(id),
      detach: () => TargetsService.Detach(),
      reload: () => TargetsService.Reload(),
      openInReactNativeDevTools: (id) => TargetsService.OpenInReactNativeDevTools(id),
    },
    console: {
      stats: () => ConsoleService.Stats(),
      // Generated enums are string unions structurally; narrow once here.
      range: async (fromSeq, limit) =>
        ((await ConsoleService.Range(fromSeq, limit)) ?? []) as unknown as ConsoleEntry[],
      clear: () => ConsoleService.Clear(),
      evaluate: async (expression) =>
        (await ConsoleService.Evaluate(expression)) as unknown as ConsoleEntry,
      properties: async (objectId, epoch) =>
        ((await ConsoleService.Properties(objectId, epoch)) ?? []) as unknown as ConsoleProperty[],
    },
    events: {
      on: (name, handler) =>
        Events.On(name, (event) => {
          handler(event.data);
        }),
    },
  };
}
