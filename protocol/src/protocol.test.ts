import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";
import { describe, expect, it } from "vitest";

import { isLensMessage, MESSAGE_TYPES, PROTOCOL_VERSION } from "./runtime";
import type { LensMessage } from "./types";

const root = resolve(__dirname, "..");
const schema = JSON.parse(
  readFileSync(resolve(root, "schema/lens-protocol.v1.schema.json"), "utf8"),
);
const ajv = new Ajv2020({ strict: true, allErrors: true });
addFormats(ajv);
const validate = ajv.compile(schema);

function fixtures(dir: string): Array<[string, unknown]> {
  const abs = resolve(root, dir);
  return readdirSync(abs)
    .filter((f) => f.endsWith(".json"))
    .map((f) => [f.replace(/\.json$/, ""), JSON.parse(readFileSync(resolve(abs, f), "utf8"))]);
}

describe("lens protocol v1", () => {
  const valid = fixtures("fixtures");

  it.each(valid)("fixture %s validates against the schema", (_name, doc) => {
    const ok = validate(doc);
    expect(ok, JSON.stringify(validate.errors)).toBe(true);
    expect(isLensMessage(doc)).toBe(true);
  });

  it.each(fixtures("fixtures/invalid"))("invalid fixture %s is rejected", (_name, doc) => {
    expect(validate(doc)).toBe(false);
  });

  it("covers every message type with a fixture", () => {
    const types = new Set(valid.map(([, doc]) => (doc as LensMessage).type));
    for (const t of MESSAGE_TYPES) expect(types.has(t), `missing fixture for ${t}`).toBe(true);
  });

  it("agrees with the schema on the version constant", () => {
    const hello = valid.find(([name]) => name === "hello")![1] as LensMessage;
    expect(hello.v).toBe(PROTOCOL_VERSION);
  });

  it("types the generated union narrowly", () => {
    const msg: LensMessage = { v: 1, type: "online.set", payload: { online: true } };
    if (msg.type === "online.set") expect(msg.payload.online).toBe(true);
  });
});
