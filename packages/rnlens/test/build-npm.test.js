"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const script = path.join(__dirname, "..", "scripts", "build-npm.js");

function fakeRelease() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "rnlens-npm-"));
  const artifacts = [];
  for (const [goos, goarch] of [
    ["darwin", "arm64"],
    ["windows", "amd64"],
  ]) {
    const dir = path.join(root, "dist", `rnlens_${goos}_${goarch}`);
    fs.mkdirSync(dir, { recursive: true });
    const name = goos === "windows" ? "rnlens.exe" : "rnlens";
    fs.writeFileSync(path.join(dir, name), `binary ${goos}/${goarch}`);
    artifacts.push({
      type: "Binary",
      goos,
      goarch,
      path: path.join("dist", `rnlens_${goos}_${goarch}`, name),
    });
  }
  artifacts.push({ type: "Archive", goos: "linux", goarch: "amd64", path: "dist/x.tar.gz" });
  fs.writeFileSync(path.join(root, "dist", "artifacts.json"), JSON.stringify(artifacts));
  return root;
}

test("writes one package per binary and the launcher, all at the release version", (t) => {
  const root = fakeRelease();
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const out = path.join(root, "npm");
  execFileSync(process.execPath, [script, "--root", root, "--out", out, "--version", "v1.2.3"]);

  assert.deepEqual(fs.readdirSync(out).sort(), ["cli-darwin-arm64", "cli-windows-amd64", "rnlens"]);

  const mac = JSON.parse(
    fs.readFileSync(path.join(out, "cli-darwin-arm64", "package.json"), "utf8"),
  );
  assert.equal(mac.name, "@rnlens/cli-darwin-arm64");
  assert.equal(mac.version, "1.2.3");
  assert.deepEqual(mac.os, ["darwin"]);
  assert.deepEqual(mac.cpu, ["arm64"]);
  const bin = path.join(out, "cli-darwin-arm64", "bin", "rnlens");
  assert.equal(fs.readFileSync(bin, "utf8"), "binary darwin/arm64");
  // Windows has no execute bits: stat reports 0 whatever chmod set.
  if (process.platform !== "win32") {
    assert.equal(fs.statSync(bin).mode & 0o111, 0o111, "the binary is executable");
  }

  const win = JSON.parse(
    fs.readFileSync(path.join(out, "cli-windows-amd64", "package.json"), "utf8"),
  );
  assert.deepEqual([win.os, win.cpu], [["win32"], ["x64"]]);
  assert.ok(fs.existsSync(path.join(out, "cli-windows-amd64", "bin", "rnlens.exe")));

  const launcher = JSON.parse(fs.readFileSync(path.join(out, "rnlens", "package.json"), "utf8"));
  assert.equal(launcher.version, "1.2.3");
  assert.deepEqual(launcher.optionalDependencies, {
    "@rnlens/cli-darwin-arm64": "1.2.3",
    "@rnlens/cli-windows-amd64": "1.2.3",
  });
  assert.ok(fs.existsSync(path.join(out, "rnlens", "bin", "rnlens.js")));
  assert.ok(fs.existsSync(path.join(out, "rnlens", "lib", "platform.js")));
  assert.ok(!fs.existsSync(path.join(out, "rnlens", "test")), "tests are not published");
});

test("refuses a version that is not semver", (t) => {
  const root = fakeRelease();
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  assert.throws(
    () =>
      execFileSync(
        process.execPath,
        [script, "--root", root, "--out", path.join(root, "npm"), "--version", "latest"],
        { stdio: "pipe" },
      ),
    /not a semver version/,
  );
});
