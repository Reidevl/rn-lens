#!/usr/bin/env node
"use strict";

// Turns a GoReleaser dist/ into npm packages ready for `npm publish`:
// @rnlens/cli-<os>-<arch> (one binary each, limited to its os/cpu) and the
// rnlens launcher pinned to the same version.
//
//   node scripts/build-npm.js --root <repo> --out <dir> --version <vX.Y.Z>
//
// It only writes files; publishing is a separate, explicit step.
const fs = require("node:fs");
const path = require("node:path");
const { GOOS, GOARCH } = require("../lib/platform.js");

function args(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i += 2) out[argv[i].replace(/^--/, "")] = argv[i + 1];
  return out;
}

/** Inverts a Node→Go name map. */
const toNode = (map) => Object.fromEntries(Object.entries(map).map(([node, go]) => [go, node]));

function main() {
  const { root = process.cwd(), out, version: raw } = args(process.argv.slice(2));
  if (!out || !raw)
    throw new Error("usage: build-npm.js --root <repo> --out <dir> --version <vX.Y.Z>");
  const version = raw.replace(/^v/, "");
  if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version))
    throw new Error(`${raw} is not a semver version`);

  const launcherDir = path.join(__dirname, "..");
  const launcher = JSON.parse(fs.readFileSync(path.join(launcherDir, "package.json"), "utf8"));
  const artifacts = JSON.parse(fs.readFileSync(path.join(root, "dist", "artifacts.json"), "utf8"));
  const nodeOS = toNode(GOOS);
  const nodeCPU = toNode(GOARCH);

  fs.rmSync(out, { recursive: true, force: true });
  const optional = {};
  for (const a of artifacts) {
    if (a.type !== "Binary" || !nodeOS[a.goos] || !nodeCPU[a.goarch]) continue;
    const pkg = `cli-${a.goos}-${a.goarch}`;
    const name = `@rnlens/${pkg}`;
    const dir = path.join(out, pkg);
    const binName = a.goos === "windows" ? "rnlens.exe" : "rnlens";
    fs.mkdirSync(path.join(dir, "bin"), { recursive: true });
    fs.copyFileSync(path.join(root, a.path), path.join(dir, "bin", binName));
    fs.chmodSync(path.join(dir, "bin", binName), 0o755);
    fs.writeFileSync(
      path.join(dir, "package.json"),
      JSON.stringify(
        {
          name,
          version,
          description: `The rnlens binary for ${a.goos}/${a.goarch}. Install rnlens instead.`,
          license: launcher.license,
          repository: launcher.repository,
          os: [nodeOS[a.goos]],
          cpu: [nodeCPU[a.goarch]],
          files: ["bin"],
          preferUnplugged: true,
        },
        null,
        2,
      ) + "\n",
    );
    fs.writeFileSync(
      path.join(dir, "README.md"),
      `# ${name}\n\nThe \`rnlens\` binary for ${a.goos}/${a.goarch}, installed by [rnlens](https://www.npmjs.com/package/rnlens). Do not depend on it directly.\n`,
    );
    optional[name] = version;
  }
  if (Object.keys(optional).length === 0)
    throw new Error("dist/artifacts.json lists no rnlens binaries");

  const dir = path.join(out, "rnlens");
  for (const sub of launcher.files)
    fs.cpSync(path.join(launcherDir, sub), path.join(dir, sub), { recursive: true });
  fs.copyFileSync(path.join(launcherDir, "README.md"), path.join(dir, "README.md"));
  const { scripts: _scripts, ...published } = launcher;
  fs.writeFileSync(
    path.join(dir, "package.json"),
    JSON.stringify({ ...published, version, optionalDependencies: optional }, null, 2) + "\n",
  );
}

try {
  main();
} catch (err) {
  console.error(`build-npm: ${err.message}`);
  process.exit(1);
}
