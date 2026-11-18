"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { platformPackage, binaryPath } = require("../lib/platform.js");

test("maps Node platforms to the GoReleaser package names", () => {
  assert.deepEqual(platformPackage("darwin", "arm64"), {
    name: "@rnlens/cli-darwin-arm64",
    binary: "bin/rnlens",
  });
  assert.deepEqual(platformPackage("win32", "x64"), {
    name: "@rnlens/cli-windows-amd64",
    binary: "bin/rnlens.exe",
  });
  assert.equal(platformPackage("linux", "ia32"), null);
  assert.equal(platformPackage("freebsd", "x64"), null);
});

test("resolves the binary inside the platform package", () => {
  const seen = [];
  const path = binaryPath({
    env: {},
    platform: "linux",
    arch: "arm64",
    resolve: (id) => {
      seen.push(id);
      return "/x/node_modules/" + id;
    },
  });
  assert.deepEqual(seen, ["@rnlens/cli-linux-arm64/bin/rnlens"]);
  assert.equal(path, "/x/node_modules/@rnlens/cli-linux-arm64/bin/rnlens");
});

test("RNLENS_BINARY overrides the lookup", () => {
  assert.equal(
    binaryPath({ env: { RNLENS_BINARY: "/opt/rnlens" }, platform: "aix", arch: "ppc" }),
    "/opt/rnlens",
  );
});

test("explains an unsupported platform and a missing package", () => {
  assert.throws(
    () => binaryPath({ env: {}, platform: "linux", arch: "ia32" }),
    /no build for linux\/ia32/,
  );
  assert.throws(
    () =>
      binaryPath({
        env: {},
        platform: "darwin",
        arch: "x64",
        resolve: () => {
          throw new Error("MODULE_NOT_FOUND");
        },
      }),
    /@rnlens\/cli-darwin-amd64 is not installed/,
  );
});
