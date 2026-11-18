// @ts-check
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  integrations: [
    starlight({
      title: "Lens",
      description:
        "React Native DevTools written in Go: console, network, queries, storage, performance and your own panels.",
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/Reidevl/rn-lens" }],
      sidebar: [
        { label: "Start here", items: ["guides/install", "guides/connect", "guides/browser-mode"] },
        {
          label: "Panels",
          items: [
            "guides/console",
            "guides/network",
            "guides/queries",
            "guides/storage",
            "guides/performance",
            "guides/plugins",
            "guides/mcp",
          ],
        },
        { label: "Use cases", items: [{ autogenerate: { directory: "use-cases" } }] },
        { label: "Reference", items: ["reference/sdk", "reference/protocol"] },
      ],
    }),
  ],
});
