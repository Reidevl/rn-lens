import { Clipboard, Events } from "@wailsio/runtime";

import * as AppService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/appservice";
import * as ConsoleService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/consoleservice";
import * as PerformanceService from "../../bindings/github.com/Reidevl/rn-lens/internal/adapters/wailsui/performanceservice";
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
  PerformanceStatus,
  Screenshot,
  QueryCounts,
  QueryItem,
  SdkDevice,
  SessionSnapshot,
  StorageDevice,
  StorageImportOutcome,
  StorageStore,
  StorageValue,
} from "./Backend";
import { fileName, pickFile, startDownload, uploadFile } from "./browserFiles";

/**
 * Resolves a call that returns nothing to undefined, as the Backend contract
 * says: over the Wails bridge a Go method without a result resolves to {},
 * which React cannot render if it ever reaches the UI.
 */
const done = (call: Promise<unknown>): Promise<void> => call.then(() => undefined);

export interface WailsBackendOptions {
  /**
   * Browser mode (the rnlens server build): saves become downloads, imports
   * start with an upload and the clipboard is the browser's.
   */
  browser?: boolean;
}

/** Backend backed by the Wails v3 bindings and event bus. */
export function createWailsBackend({ browser = false }: WailsBackendOptions = {}): Backend {
  /**
   * In browser mode Go wrote the file to a one-time download URL: start the
   * download and report the file name instead of a server path.
   */
  const delivered = async (path: string): Promise<string> => {
    if (!browser || !path) return path;
    const url = await AppService.DownloadURL(path);
    if (!url) return path;
    const name = fileName(path);
    startDownload(url, name);
    return name;
  };
  const save = (call: () => Promise<string>) => async () => delivered(await call());

  return {
    kind: "wails",
    app: {
      info: () => AppService.Info(),
    },
    targets: {
      list: async () => (await TargetsService.List()) ?? [],
      refresh: () => done(TargetsService.Refresh()),
      // The generated enum is structurally a string union; narrow it once here.
      session: async () => (await TargetsService.Session()) as unknown as SessionSnapshot,
      attach: (id) => done(TargetsService.Attach(id)),
      detach: () => done(TargetsService.Detach()),
      reload: () => done(TargetsService.Reload()),
      openInReactNativeDevTools: (id) => done(TargetsService.OpenInReactNativeDevTools(id)),
    },
    console: {
      stats: () => ConsoleService.Stats(),
      // Generated enums are string unions structurally; narrow once here.
      range: async (fromSeq, limit) =>
        ((await ConsoleService.Range(fromSeq, limit)) ?? []) as unknown as ConsoleEntry[],
      clear: () => done(ConsoleService.Clear()),
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
        done(QueriesService.Action(deviceKey, type, queryHash, dataJSON)),
      setOnline: (deviceKey, online) => done(QueriesService.SetOnline(deviceKey, online)),
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
      clear: () => done(NetworkService.Clear()),
      saveHAR: save(() => NetworkService.SaveHAR()),
    },
    storage: {
      devices: async () => ((await StorageService.Devices()) ?? []) as unknown as StorageDevice[],
      // Generated enums are string unions structurally; narrow once here.
      stores: async (deviceKey) =>
        ((await StorageService.Stores(deviceKey)) ?? []) as unknown as StorageStore[],
      get: async (deviceKey, storeId, key) =>
        (await StorageService.Get(deviceKey, storeId, key)) as unknown as StorageValue,
      set: (deviceKey, storeId, key, type, value) =>
        done(StorageService.Set(deviceKey, storeId, key, type, value)),
      remove: (deviceKey, storeId, key) => done(StorageService.Remove(deviceKey, storeId, key)),
      clear: (deviceKey, storeId) => done(StorageService.Clear(deviceKey, storeId)),
      refresh: (deviceKey, storeId) => done(StorageService.Refresh(deviceKey, storeId)),
      exportStore: async (deviceKey, storeId) =>
        delivered(await StorageService.Export(deviceKey, storeId)),
      importStore: async (deviceKey, storeId) => {
        if (!browser) {
          return (await StorageService.Import(
            deviceKey,
            storeId,
          )) as unknown as StorageImportOutcome;
        }
        const file = await pickFile(".json,application/json");
        if (!file) return { path: "", written: 0, skipped: null };
        const token = await uploadFile(file);
        return (await StorageService.ImportUpload(
          deviceKey,
          storeId,
          token,
          file.name,
        )) as unknown as StorageImportOutcome;
      },
    },
    performance: {
      // time.Time and the phase enum arrive structurally typed; narrow once here.
      status: async () => (await PerformanceService.Status()) as unknown as PerformanceStatus,
      traceCategories: async () => (await PerformanceService.TraceCategories()) ?? [],
      screenshot: async (format, quality) =>
        (await PerformanceService.Screenshot(format, quality)) as unknown as Screenshot,
      saveScreenshot: save(() => PerformanceService.SaveScreenshot()),
      startTrace: (categories) => done(PerformanceService.StartTrace(categories)),
      stopTrace: () => PerformanceService.StopTrace(),
      saveTrace: save(() => PerformanceService.SaveTrace()),
      startProfile: () => done(PerformanceService.StartProfile()),
      stopProfile: () => PerformanceService.StopProfile(),
      saveProfile: save(() => PerformanceService.SaveProfile()),
      saveHeapSnapshot: async () => {
        const result = await PerformanceService.SaveHeapSnapshot();
        return { ...result, path: await delivered(result.path) };
      },
    },
    clipboard: {
      // The webview's navigator.clipboard is not reliable under the Wails
      // custom scheme; the runtime writes through the native pasteboard.
      // SetText drops the OS result, so read the text back to confirm it.
      writeText: async (text) => {
        if (browser) {
          // A browser tab on localhost is a secure context.
          await navigator.clipboard.writeText(text);
          return;
        }
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
