import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { afterEach, describe, expect, it, vi } from "vitest";

import { defaultEnabled, hostFromUrl, readRuntimeEnv, resolveLensUrl } from "./env";
import { SDK_VERSION } from "./version";

describe("resolveLensUrl", () => {
  it("uses the Metro bundle host", () => {
    expect(
      resolveLensUrl({
        platform: "ios",
        scriptURL: "http://192.168.1.10:8081/index.bundle?platform=ios&dev=true",
      }),
    ).toBe("ws://192.168.1.10:7420/lens");
  });

  it("maps loopback to 10.0.2.2 on Android only", () => {
    const scriptURL = "http://localhost:8081/index.bundle";
    expect(resolveLensUrl({ platform: "android", scriptURL })).toBe("ws://10.0.2.2:7420/lens");
    expect(
      resolveLensUrl({ platform: "android", scriptURL: "http://127.0.0.1:8081/index.bundle" }),
    ).toBe("ws://10.0.2.2:7420/lens");
    expect(resolveLensUrl({ platform: "ios", scriptURL })).toBe("ws://localhost:7420/lens");
  });

  it("keeps a LAN host on Android", () => {
    expect(
      resolveLensUrl({ platform: "android", scriptURL: "http://10.0.0.5:8081/index.bundle" }),
    ).toBe("ws://10.0.0.5:7420/lens");
  });

  it("falls back to the browser hostname, then localhost", () => {
    expect(resolveLensUrl({ platform: "web", browserHostname: "dev.local" })).toBe(
      "ws://dev.local:7420/lens",
    );
    expect(resolveLensUrl({ platform: "other" })).toBe("ws://localhost:7420/lens");
  });

  it("accepts a custom port", () => {
    expect(resolveLensUrl({ platform: "other" }, 9000)).toBe("ws://localhost:9000/lens");
  });
});

describe("hostFromUrl", () => {
  it("parses hosts with ports, credentials and IPv6", () => {
    expect(hostFromUrl("http://host:8081/x")).toBe("host");
    expect(hostFromUrl("http://user:pw@host/x")).toBe("host");
    expect(hostFromUrl("http://[::1]:8081/x")).toBe("[::1]");
    expect(hostFromUrl("not a url")).toBeUndefined();
    expect(hostFromUrl(undefined)).toBeUndefined();
  });
});

describe("readRuntimeEnv", () => {
  it("is 'other' outside React Native and a browser", () => {
    const env = readRuntimeEnv();
    expect(env.platform).toBe("other");
    expect(env.scriptURL).toBeUndefined();
  });
});

describe("defaultEnabled", () => {
  const g = globalThis as { __DEV__?: boolean };
  afterEach(() => {
    delete g.__DEV__;
    vi.unstubAllEnvs();
  });

  it("follows __DEV__ when defined", () => {
    g.__DEV__ = false;
    expect(defaultEnabled()).toBe(false);
    g.__DEV__ = true;
    expect(defaultEnabled()).toBe(true);
  });

  it("falls back to NODE_ENV", () => {
    vi.stubEnv("NODE_ENV", "production");
    expect(defaultEnabled()).toBe(false);
    vi.stubEnv("NODE_ENV", "development");
    expect(defaultEnabled()).toBe(true);
  });
});

describe("SDK_VERSION", () => {
  it("matches package.json", () => {
    const pkg = JSON.parse(readFileSync(resolve(__dirname, "../package.json"), "utf8"));
    expect(SDK_VERSION).toBe(pkg.version);
  });
});
