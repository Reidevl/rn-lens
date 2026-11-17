import type { Backend } from "./Backend";
import { createMockBackend } from "./mock";

export type { AppInfo, Backend, Unsubscribe } from "./Backend";
export { BackendProvider } from "./BackendProvider";
export { createMockBackend, type MockBackend } from "./mock";
export { useBackend } from "./useBackend";

/**
 * Picks the Wails transport inside the desktop shell and the mock elsewhere.
 * The Wails runtime is imported lazily so browser previews and tests never
 * load it.
 */
export async function detectBackend(): Promise<Backend> {
  const runningInWails = typeof window !== "undefined" && "_wails" in window;
  if (!runningInWails) return createMockBackend();

  const { createWailsBackend } = await import("./wails");
  return createWailsBackend();
}
