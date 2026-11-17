/// <reference types="vitest/config" />
import { mkdirSync, writeFileSync } from "node:fs";

import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

/**
 * Keeps dist/.gitkeep after every build. The Go embed directive
 * (`//go:embed all:dist`) needs at least one file in dist for test-only
 * builds, and Vite empties the directory before writing the bundle.
 */
function keepDistPlaceholder(): Plugin {
  return {
    name: "lens:keep-dist-placeholder",
    closeBundle() {
      mkdirSync("dist", { recursive: true });
      writeFileSync("dist/.gitkeep", "");
    },
  };
}

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), tailwindcss(), wails("./bindings"), keepDistPlaceholder()],
  resolve: {
    alias: { "@": new URL("./src", import.meta.url).pathname },
  },
  test: {
    environment: "jsdom",
    globals: false,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
  },
});
