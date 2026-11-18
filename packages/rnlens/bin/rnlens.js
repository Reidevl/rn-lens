#!/usr/bin/env node
"use strict";

// Runs the rnlens binary for this platform with the same arguments, then
// exits with its exit code. Ctrl+C reaches the binary directly (same
// terminal process group); other signals are forwarded.
const { spawn } = require("node:child_process");
const { binaryPath } = require("../lib/platform.js");

let bin;
try {
  bin = binaryPath();
} catch (err) {
  console.error(err.message);
  process.exit(1);
}

const child = spawn(bin, process.argv.slice(2), { stdio: "inherit" });
for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
  process.on(signal, () => child.kill(signal));
}
child.on("error", (err) => {
  console.error(`rnlens: cannot run ${bin}: ${err.message}`);
  process.exit(1);
});
child.on("exit", (code, signal) => {
  process.exit(code ?? (signal ? 1 : 0));
});
