import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { compile } from "json-schema-to-typescript";

const here = dirname(fileURLToPath(import.meta.url));
const schemaPath = resolve(here, "../schema/lens-protocol.v1.schema.json");
const outPath = resolve(here, "../src/types.ts");

const schema = JSON.parse(readFileSync(schemaPath, "utf8"));
const ts = await compile(schema, "LensMessage", {
  bannerComment:
    "/* eslint-disable */\n/**\n * Generated from protocol/schema/lens-protocol.v1.schema.json by `pnpm --filter @rnlens/protocol run generate`.\n * Do not edit by hand.\n */",
  additionalProperties: false,
  strictIndexSignatures: true,
  style: { printWidth: 100 },
});
mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, ts);
console.log(`wrote ${outPath} (${ts.length} bytes)`);
