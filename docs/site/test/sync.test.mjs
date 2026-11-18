import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { convert, REPO, sync } from "../scripts/sync.mjs";

const map = new Map([
  ["docs/guides/console.md", "guides/console"],
  ["docs/guides/network.md", "guides/network"],
  ["protocol/README.md", "reference/protocol"],
]);

test("lifts the H1 into frontmatter and drops it from the body", () => {
  const out = convert("# Console\n\nThe console.\n", "docs/guides/console.md", map);
  assert.ok(out.startsWith('---\ntitle: "Console"\n'));
  assert.ok(out.includes(`editUrl: ${REPO.replace("/blob/", "/edit/")}/docs/guides/console.md`));
  assert.ok(out.endsWith("---\n\nThe console.\n"));
  assert.ok(!out.includes("# Console"));
});

test("links synced pages on the site and everything else on GitHub", () => {
  const md = [
    "# Network",
    "See [console](console.md#eval), [protocol](../../protocol/README.md),",
    "[the example](../../examples/expo-app/App.tsx), [web](https://x.dev/a.md), [here](#the-table).",
  ].join("\n");
  const out = convert(md, "docs/guides/network.md", map);
  assert.ok(out.includes("[console](/guides/console/#eval)"));
  assert.ok(out.includes("[protocol](/reference/protocol/)"));
  assert.ok(out.includes(`[the example](${REPO}/examples/expo-app/App.tsx)`));
  assert.ok(out.includes("[web](https://x.dev/a.md)"));
  assert.ok(out.includes("[here](#the-table)"));
});

test("refuses a page without a title", () => {
  assert.throws(() => convert("no title\n", "docs/guides/x.md", map), /no "# Title" line/);
});

test("writes every guide and the references, replacing stale pages", (t) => {
  const root = mkdtempSync(join(tmpdir(), "lens-site-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "docs/guides"), { recursive: true });
  mkdirSync(join(root, "protocol"), { recursive: true });
  mkdirSync(join(root, "packages/sdk"), { recursive: true });
  writeFileSync(
    join(root, "docs/guides/install.md"),
    "# Install\n\nSee [the SDK](../../packages/sdk/README.md).\n",
  );
  writeFileSync(join(root, "protocol/README.md"), "# Protocol\n");
  writeFileSync(join(root, "packages/sdk/README.md"), "# SDK\n");
  const out = join(root, "site");
  mkdirSync(join(out, "guides"), { recursive: true });
  writeFileSync(join(out, "guides/removed.md"), "stale");
  mkdirSync(join(out, "use-cases"), { recursive: true });
  writeFileSync(join(out, "use-cases/kept.md"), "hand-written");

  sync(root, out);
  assert.ok(
    readFileSync(join(out, "guides/install.md"), "utf8").includes("[the SDK](/reference/sdk/)"),
  );
  assert.ok(readFileSync(join(out, "reference/protocol.md"), "utf8").includes('title: "Protocol"'));
  assert.throws(() => readFileSync(join(out, "guides/removed.md")), /ENOENT/);
  assert.equal(readFileSync(join(out, "use-cases/kept.md"), "utf8"), "hand-written");
});
