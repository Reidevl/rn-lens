import { describe, expect, it } from "vitest";

import { safeSerialize, toSafeJson } from "./serialize";

describe("toSafeJson", () => {
  it("converts BigInt to string and Date to ISO", () => {
    const date = new Date("2026-01-02T03:04:05.000Z");
    expect(toSafeJson({ big: 10n, date })).toEqual({ big: "10", date: "2026-01-02T03:04:05.000Z" });
  });

  it("omits undefined, marks functions and symbols", () => {
    const out = toSafeJson({ a: undefined, fn: () => 1, sym: Symbol("x"), nested: [undefined] });
    expect(out).toEqual({ fn: "[Function]", sym: "Symbol(x)", nested: [null] });
    expect(JSON.stringify(out)).toBe('{"fn":"[Function]","sym":"Symbol(x)","nested":[null]}');
  });

  it("marks circular references but keeps repeated references", () => {
    const shared = { id: 1 };
    const obj: Record<string, unknown> = { shared, again: shared };
    obj.self = obj;
    expect(toSafeJson(obj)).toEqual({ shared: { id: 1 }, again: { id: 1 }, self: "[Circular]" });
  });

  it("converts Map, Set, Error and typed arrays", () => {
    const err = new Error("boom");
    const out = toSafeJson({
      map: new Map([["k", 1]]),
      set: new Set([1, 2]),
      err,
      bytes: new Uint8Array([1, 2]),
    }) as Record<string, unknown>;
    expect(out.map).toEqual([["k", 1]]);
    expect(out.set).toEqual([1, 2]);
    expect(out.err).toEqual({ name: "Error", message: "boom", stack: err.stack });
    expect(out.bytes).toEqual([1, 2]);
  });

  it("stops at maxDepth", () => {
    const deep = { a: { b: { c: { d: 1 } } } };
    expect(toSafeJson(deep, 2)).toEqual({ a: { b: "[MaxDepth]" } });
  });

  it("stringifies non-finite numbers", () => {
    expect(toSafeJson([NaN, Infinity])).toEqual(["NaN", "Infinity"]);
  });
});

describe("safeSerialize", () => {
  it("returns the value with its byte size", () => {
    const result = safeSerialize({ a: "é" });
    expect(result).toEqual({ value: { a: "é" }, truncated: false, bytes: 10 });
  });

  it("replaces oversized values with the truncation marker", () => {
    const result = safeSerialize({ blob: "x".repeat(100) }, { maxBytes: 50 });
    expect(result.truncated).toBe(true);
    expect(result.bytes).toBe(111);
    expect(result.value).toEqual({ __lensTruncated: true, bytes: 111 });
  });

  it("keeps undefined as undefined", () => {
    expect(safeSerialize(undefined)).toEqual({ value: undefined, truncated: false, bytes: 0 });
  });

  it("never throws on circular input", () => {
    const obj: Record<string, unknown> = {};
    obj.me = obj;
    expect(() => safeSerialize(obj)).not.toThrow();
    expect(safeSerialize(obj).value).toEqual({ me: "[Circular]" });
  });
});
