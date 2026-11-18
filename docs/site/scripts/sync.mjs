// Copies the Markdown that lives next to the code (docs/guides, the protocol
// and SDK READMEs) into the Starlight content folder, so the site never has
// a second copy to keep in sync. Each page gets its H1 as frontmatter title;
// links to other synced pages become site links, every other relative link
// points at GitHub.
//
//   node scripts/sync.mjs            (run by `pnpm build` and `pnpm dev`)
import { mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, normalize, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const REPO = "https://github.com/Reidevl/rn-lens/blob/main";

/** Source file (relative to the repository) → site slug. */
export function pages(root) {
  const guides = readdirSync(join(root, "docs/guides"))
    .filter((f) => f.endsWith(".md"))
    .map((f) => [`docs/guides/${f}`, `guides/${f.replace(/\.md$/, "")}`]);
  return new Map([
    ...guides,
    ["protocol/README.md", "reference/protocol"],
    ["packages/sdk/README.md", "reference/sdk"],
  ]);
}

/** Rewrites one Markdown file for the site. */
export function convert(markdown, source, map) {
  const lines = markdown.split("\n");
  const h1 = lines.findIndex((l) => l.startsWith("# "));
  if (h1 < 0) throw new Error(`${source}: no "# Title" line`);
  const title = lines[h1].slice(2).trim();
  lines.splice(h1, 1);
  const from = dirname(source);
  const body = lines
    .join("\n")
    .replace(/^\n+/, "")
    .replace(/\]\(([^)\s]+)\)/g, (match, href) => {
      if (/^[a-z]+:|^#|^\//i.test(href)) return match;
      const [path, hash = ""] = href.split("#");
      const target = normalize(join(from, path));
      const slug = map.get(target);
      const anchor = hash ? `#${hash}` : "";
      if (slug) return `](/${slug}/${anchor})`;
      return `](${REPO}/${target}${anchor})`;
    });
  return `---\ntitle: ${JSON.stringify(title)}\neditUrl: ${REPO.replace("/blob/", "/edit/")}/${source}\n---\n\n${body}`;
}

export function sync(root, out) {
  const map = pages(root);
  for (const slug of new Set([...map.values()].map((s) => s.split("/")[0]))) {
    rmSync(join(out, slug), { recursive: true, force: true });
  }
  for (const [source, slug] of map) {
    const target = join(out, `${slug}.md`);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, convert(readFileSync(join(root, source), "utf8"), source, map));
  }
  return map;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const here = dirname(fileURLToPath(import.meta.url));
  const root = resolve(here, "../../..");
  const out = resolve(here, "../src/content/docs");
  const map = sync(root, out);
  console.log(`synced ${map.size} pages into ${relative(process.cwd(), out)}`);
}
