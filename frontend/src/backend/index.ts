import type { Backend } from "./Backend";
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
  Metadata,
  MutationItem,
  ObjectPreview,
  QueryActionType,
  QueryCounts,
  QueryItem,
  QueryLabel,
  QueryState,
  SdkDevice,
  RemoteObject,
  SessionSnapshot,
  SessionState,
  StackTrace,
  Target,
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
  if (!runningInWails) return createMockBackend({ targets: [sampleTarget()] });

  const { createWailsBackend } = await import("./wails");
  return createWailsBackend();
}
