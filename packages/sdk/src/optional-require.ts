/* eslint-disable @typescript-eslint/no-require-imports -- optional dependency probes */
/**
 * Optional modules are loaded with literal `require` calls wrapped in
 * try/catch: Metro resolves them statically and, with its default
 * `allowOptionalDependencies`, tolerates a missing package. In an ESM runtime
 * without `require` (a plain browser bundle) every probe resolves to
 * `undefined` and the SDK falls back to its defaults.
 */

function hasRequire(): boolean {
  return typeof require === "function";
}

export function loadReactNative<T>(): T | undefined {
  if (!hasRequire()) return undefined;
  try {
    return require("react-native") as T;
  } catch {
    return undefined;
  }
}

export function loadReactQuery<T>(): T | undefined {
  if (!hasRequire()) return undefined;
  try {
    return require("@tanstack/react-query") as T;
  } catch {
    return undefined;
  }
}

export function loadAsyncStorage<T>(): T | undefined {
  if (!hasRequire()) return undefined;
  try {
    return require("@react-native-async-storage/async-storage") as T;
  } catch {
    return undefined;
  }
}
