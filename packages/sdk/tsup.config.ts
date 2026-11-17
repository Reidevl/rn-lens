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
  treeshake: true,
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
