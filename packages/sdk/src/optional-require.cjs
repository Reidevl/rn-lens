/* eslint-disable -- CommonJS on purpose, see the note below */
/**
 * Optional modules are loaded with literal `require` calls wrapped in
 * try/catch: Metro resolves them statically and, with its default
 * `allowOptionalDependencies`, tolerates a missing package.
 *
 * This file is CommonJS by design. Inside an ES module esbuild rewrites
 * `require(...)` into a `__require` shim that Metro cannot resolve, and every
 * probe then fails with `Requiring unknown module`. A CommonJS source keeps the
 * calls literal in the CJS build, which is what the `react-native` export
 * condition serves. The ESM build still receives the shim: without a global
 * `require` (a plain browser bundle) every probe falls back to `undefined`.
 * `scripts/check-dist.mjs` guards this after every build.
 */

function hasRequire() {
  return typeof require === "function";
}

function loadReactNative() {
  if (!hasRequire()) return undefined;
  try {
    return require("react-native");
  } catch {
    return undefined;
  }
}

function loadReactQuery() {
  if (!hasRequire()) return undefined;
  try {
    return require("@tanstack/react-query");
  } catch {
    return undefined;
  }
}

function loadAsyncStorage() {
  if (!hasRequire()) return undefined;
  try {
    return require("@react-native-async-storage/async-storage");
  } catch {
    return undefined;
  }
}

module.exports = { loadReactNative, loadReactQuery, loadAsyncStorage };
