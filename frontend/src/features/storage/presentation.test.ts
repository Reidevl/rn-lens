import { describe, expect, it } from "vitest";

import { bytesOf, hexDump, jsonOf, toBase64, valueSize } from "./presentation";

const text = (value: string) => ({ type: "string" as const, value });

describe("storage value views", () => {
  it("reads the bytes of text as UTF-8 and of bytes values from base64", () => {
    expect([...bytesOf(text("hé"))]).toEqual([0x68, 0xc3, 0xa9]);
    expect([...bytesOf({ type: "bytes", value: "AAH/" })]).toEqual([0, 1, 255]);
    expect([...bytesOf({ type: "number", value: "42" })]).toEqual([0x34, 0x32]);
  });

  it("dumps bytes as offset, hex and printable ASCII", () => {
    expect(hexDump(text("Hello, Lens!"))).toBe(
      "00000000  48 65 6c 6c 6f 2c 20 4c 65 6e 73 21              |Hello, Lens!|",
    );
    const lines = hexDump({
      type: "bytes",
      value: btoa(String.fromCharCode(...Array.from({ length: 20 }, (_, i) => i))),
    }).split("\n");
    expect(lines).toHaveLength(2);
    expect(lines[1].startsWith("00000010  10 11 12 13")).toBe(true);
    expect(lines[0].endsWith("|................|")).toBe(true);
  });

  it("encodes text to base64 and keeps bytes as they are", () => {
    expect(toBase64(text("hé"))).toBe("aMOp");
    expect(toBase64({ type: "bytes", value: "AAH/" })).toBe("AAH/");
  });

  it("parses JSON only from strings that hold JSON objects or arrays", () => {
    expect(jsonOf(text('{"a":[1,2]}'))).toEqual({ ok: true, value: { a: [1, 2] } });
    expect(jsonOf(text("plain"))).toEqual({ ok: false });
    expect(jsonOf(text("42"))).toEqual({ ok: false });
    expect(jsonOf({ type: "bytes", value: "e30=" })).toEqual({ ok: false });
  });

  it("measures values in bytes", () => {
    expect(valueSize(text("hé"))).toBe(3);
    expect(valueSize({ type: "bytes", value: "AAH/" })).toBe(3);
    expect(valueSize({ type: "string", value: "", truncated: true, size: 2_000_000 })).toBe(
      2_000_000,
    );
  });
});
