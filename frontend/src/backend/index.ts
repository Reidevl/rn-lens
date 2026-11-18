import type { Backend } from "./Backend";
import { detectMode } from "./browserFiles";
import { createMockBackend, sampleTarget } from "./mock";

export type {
  AppInfo,
  Backend,
  CallFrame,
  Capabilities,
  ConsoleEntry,
  ConsoleKind,
  ConsoleLevel,
  ConsoleProperty,
  ConsoleSource,
  ConsoleStats,
  MCPInfo,
  McpCall,
  McpStatus,
  McpTool,
  McpToolKind,
  FastRefreshPhase,
  FastRefreshState,
  HeapSnapshotResult,
  Metadata,
  MutationItem,
  NetworkAvailability,
  NetworkBody,
  NetworkInitiator,
  NetworkRequest,
  NetworkState,
  NetworkStats,
  NetworkSummary,
  NetworkTiming,
  ObjectPreview,
  PerformanceStatus,
  PluginBlock,
  PluginButton,
  PluginDevice,
  PluginInfo,
  ProfileSummary,
  QueryActionType,
  QueryCounts,
  QueryItem,
  QueryLabel,
  QueryState,
  SdkDevice,
  Screenshot,
  ScreenshotFormat,
  RemoteObject,
  SessionSnapshot,
  SessionState,
  StorageDevice,
  StorageImportOutcome,
  StorageKeyInfo,
  StorageKind,
  StorageStore,
  StorageValue,
  StorageValueType,
  StackTrace,
  Target,
  TraceSummary,
  Unsubscribe,
} from "./Backend";
export { TOPICS } from "./Backend";
export { BackendProvider } from "./BackendProvider";
export {
  createMockBackend,
  idleSession,
  sampleDevice,
  sampleQuery,
  sampleTarget,
  type MockBackend,
} from "./mock";
export { useBackend } from "./useBackend";

/**
 * Picks the Wails transport inside the desktop shell and the mock elsewhere.
 * The Wails runtime is imported lazily so browser previews and tests never
 * load it. The browser mock is seeded with one target so panels can be
 * previewed with `pnpm dev`.
 */
export async function detectBackend(): Promise<Backend> {
  const runningInWails = typeof window !== "undefined" && "_wails" in window;
  if (runningInWails) {
    const { createWailsBackend } = await import("./wails");
    return createWailsBackend();
  }
  // The rnlens server build serves this page to a browser tab.
  if ((await detectMode()) === "browser") {
    const { createWailsBackend } = await import("./wails");
    return createWailsBackend({ browser: true });
  }
  return createMockBackend({ targets: [sampleTarget()] });
}
