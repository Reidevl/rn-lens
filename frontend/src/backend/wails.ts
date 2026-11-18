import { Clipboard, Events } from "@wailsio/runtime";

import * as AppService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/appservice";
import * as ConsoleService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/consoleservice";
import * as NetworkService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/networkservice";
import * as StorageService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/storageservice";
import * as QueriesService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/queriesservice";
import * as TargetsService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/targetsservice";
import type {
  Backend,
  ConsoleEntry,
  ConsoleProperty,
  NetworkRequest,
  NetworkSummary,
  MutationItem,
  QueryCounts,
  QueryItem,
  SdkDevice,
  SessionSnapshot,
  StorageDevice,
  StorageImportOutcome,
  StorageStore,
  StorageValue,
} from "./Backend";

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
    queries: {
      sdkAddr: () => QueriesService.SDKAddr(),
      devices: async () => ((await QueriesService.Devices()) ?? []) as unknown as SdkDevice[],
      queries: async (deviceKey) => {
        const page = await QueriesService.Queries(deviceKey);
        return {
          queries: (page.queries ?? []) as unknown as QueryItem[],
          counts: page.counts as unknown as QueryCounts,
        };
      },
      query: async (deviceKey, queryHash) =>
        (await QueriesService.Query(deviceKey, queryHash)) as unknown as QueryItem,
      mutations: async (deviceKey) =>
        ((await QueriesService.Mutations(deviceKey)) ?? []) as unknown as MutationItem[],
      action: (deviceKey, type, queryHash, dataJSON) =>
        QueriesService.Action(deviceKey, type, queryHash, dataJSON),
      setOnline: (deviceKey, online) => QueriesService.SetOnline(deviceKey, online),
    },
    network: {
      availability: () => NetworkService.Availability(),
      stats: () => NetworkService.Stats(),
      // Generated enums are string unions structurally; narrow once here.
      since: async (version, limit) =>
        ((await NetworkService.Since(version, limit)) ?? []) as unknown as NetworkSummary[],
      request: async (seq) => (await NetworkService.Request(seq)) as unknown as NetworkRequest,
      responseBody: (seq, epoch) => NetworkService.ResponseBody(seq, epoch),
      requestBody: (seq, epoch) => NetworkService.RequestBody(seq, epoch),
      clear: () => NetworkService.Clear(),
      saveHAR: () => NetworkService.SaveHAR(),
    },
    storage: {
      devices: async () => ((await StorageService.Devices()) ?? []) as unknown as StorageDevice[],
      // Generated enums are string unions structurally; narrow once here.
      stores: async (deviceKey) =>
        ((await StorageService.Stores(deviceKey)) ?? []) as unknown as StorageStore[],
      get: async (deviceKey, storeId, key) =>
        (await StorageService.Get(deviceKey, storeId, key)) as unknown as StorageValue,
      set: (deviceKey, storeId, key, type, value) =>
        StorageService.Set(deviceKey, storeId, key, type, value),
      remove: (deviceKey, storeId, key) => StorageService.Remove(deviceKey, storeId, key),
      clear: (deviceKey, storeId) => StorageService.Clear(deviceKey, storeId),
      refresh: (deviceKey, storeId) => StorageService.Refresh(deviceKey, storeId),
      exportStore: (deviceKey, storeId) => StorageService.Export(deviceKey, storeId),
      importStore: async (deviceKey, storeId) =>
        (await StorageService.Import(deviceKey, storeId)) as unknown as StorageImportOutcome,
    },
    clipboard: {
      // The webview's navigator.clipboard is not reliable under the Wails
      // custom scheme; the runtime writes through the native pasteboard.
      // SetText drops the OS result, so read the text back to confirm it.
      writeText: async (text) => {
        await Clipboard.SetText(text);
        if ((await Clipboard.Text()) !== text) {
          throw new Error("clipboard: the system refused the text");
        }
      },
    },
    events: {
      on: (name, handler) =>
        Events.On(name, (event) => {
          handler(event.data);
        }),
    },
  };
}
