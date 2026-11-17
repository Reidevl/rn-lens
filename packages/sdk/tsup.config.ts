import { defineConfig } from "tsup";

export default defineConfig({
  entry: { index: "src/index.ts" },
  format: ["esm", "cjs"],
  // Inline the private protocol package types into the published d.ts.
  dts: { resolve: true },
  sourcemap: true,
  clean: true,
  target: "es2020",
  platform: "neutral",
  // tsup's extra Rollup tree-shaking pass rewrites `require` into an esbuild
  // `__require` shim that Metro cannot resolve; esbuild's own tree shaking is
  // enough. See src/optional-require.cjs and scripts/check-dist.mjs.
  treeshake: false,
  // The protocol package is private to the workspace; its runtime constants and
  // types are inlined into the published bundle.
  noExternal: ["@rnlens/protocol"],
  external: [
    "react",
    "react-native",
    "@tanstack/react-query",
    "@react-native-async-storage/async-storage",
  ],
});
