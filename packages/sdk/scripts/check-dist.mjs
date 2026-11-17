// Fails the build when the CJS bundle no longer carries literal `require`
// calls for the optional peers. Metro resolves dependencies statically, so an
// esbuild `__require` shim would make every optional probe fail at runtime.
import { readFileSync } from "node:fs";

const OPTIONAL_PEERS = [
  "react-native",
  "@tanstack/react-query",
  "@react-native-async-storage/async-storage",
];

const cjs = readFileSync(new URL("../dist/index.cjs", import.meta.url), "utf8");
const problems = [];
for (const name of OPTIONAL_PEERS) {
  if (!cjs.includes(`require("${name}")`)) problems.push(`missing literal require("${name}")`);
}
if (/__require\("/.test(cjs)) problems.push("found an esbuild __require shim call");

if (problems.length > 0) {
  console.error(`dist/index.cjs is not Metro-safe:\n- ${problems.join("\n- ")}`);
  process.exit(1);
}
console.log("dist/index.cjs: optional requires are literal (Metro-safe)");
