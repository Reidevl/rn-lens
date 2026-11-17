import type { AppInfo } from "@rnlens/protocol";

import { loadReactNative } from "./optional-require";

export const DEFAULT_LENS_PORT = 7420;
export const DEFAULT_LENS_PATH = "/lens";

/** What the SDK knows about the runtime it runs in; every field is best effort. */
export interface RuntimeEnv {
  platform: AppInfo["platform"];
  rnVersion?: string;
  /** Metro bundle URL the app loaded, when running under React Native. */
  scriptURL?: string;
  /** Device model when React Native exposes it. */
  deviceModel?: string;
  /** `location.hostname` when running in a browser. */
  browserHostname?: string;
}

interface ReactNativeLike {
  Platform?: {
    OS?: string;
    constants?: {
      reactNativeVersion?: {
        major: number;
        minor: number;
        patch: number;
        prerelease?: string | null;
      };
      Model?: string;
      systemName?: string;
    };
  };
  NativeModules?: {
    SourceCode?: { scriptURL?: string; getConstants?: () => { scriptURL?: string } | undefined };
  };
  TurboModuleRegistry?: {
    get?: (name: string) => { getConstants?: () => { scriptURL?: string } | undefined } | null;
  };
}

const PLATFORMS: ReadonlySet<string> = new Set(["ios", "android", "web", "macos", "windows"]);

function toPlatform(os: string | undefined): AppInfo["platform"] {
  return os !== undefined && PLATFORMS.has(os) ? (os as AppInfo["platform"]) : "other";
}

function readScriptURL(rn: ReactNativeLike): string | undefined {
  try {
    const sourceCode = rn.NativeModules?.SourceCode;
    const direct = sourceCode?.scriptURL ?? sourceCode?.getConstants?.()?.scriptURL;
    if (typeof direct === "string" && direct.length > 0) return direct;
    const turbo = rn.TurboModuleRegistry?.get?.("SourceCode")?.getConstants?.()?.scriptURL;
    return typeof turbo === "string" && turbo.length > 0 ? turbo : undefined;
  } catch {
    return undefined;
  }
}

/** Reads platform, React Native version, Metro script URL and device model defensively. */
export function readRuntimeEnv(): RuntimeEnv {
  const env: RuntimeEnv = { platform: "other" };
  const rn = loadReactNative<ReactNativeLike>();
  if (rn?.Platform) {
    env.platform = toPlatform(rn.Platform.OS);
    const version = rn.Platform.constants?.reactNativeVersion;
    if (version) env.rnVersion = `${version.major}.${version.minor}.${version.patch}`;
    const model = rn.Platform.constants?.Model ?? rn.Platform.constants?.systemName;
    if (typeof model === "string") env.deviceModel = model;
    env.scriptURL = readScriptURL(rn);
  }
  const location = (globalThis as { location?: { hostname?: string } }).location;
  if (typeof location?.hostname === "string" && location.hostname.length > 0) {
    env.browserHostname = location.hostname;
    if (env.platform === "other") env.platform = "web";
  }
  return env;
}

/** Extracts the host of a URL without relying on the runtime's URL implementation. */
export function hostFromUrl(url: string | undefined): string | undefined {
  if (!url) return undefined;
  const match = /^[a-z][a-z0-9+.-]*:\/\/(?:[^@/]*@)?(\[[^\]]+\]|[^/:?#]+)/i.exec(url);
  return match?.[1];
}

const LOOPBACK: ReadonlySet<string> = new Set(["localhost", "127.0.0.1", "[::1]"]);

/**
 * Derives the Lens URL: the host of the Metro bundle, then the browser host,
 * then localhost. Android emulators reach the host machine through 10.0.2.2.
 */
export function resolveLensUrl(
  env: Pick<RuntimeEnv, "platform" | "scriptURL" | "browserHostname">,
  port: number = DEFAULT_LENS_PORT,
): string {
  let host = hostFromUrl(env.scriptURL) ?? env.browserHostname ?? "localhost";
  if (env.platform === "android" && LOOPBACK.has(host)) host = "10.0.2.2";
  return `ws://${host}:${port}${DEFAULT_LENS_PATH}`;
}

/** Default `enabled`: `__DEV__` under React Native, otherwise NODE_ENV !== "production". */
export function defaultEnabled(): boolean {
  const dev = (globalThis as { __DEV__?: boolean }).__DEV__;
  if (typeof dev === "boolean") return dev;
  const nodeEnv = typeof process !== "undefined" && process.env ? process.env.NODE_ENV : undefined;
  return nodeEnv !== "production";
}
