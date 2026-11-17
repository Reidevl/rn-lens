export interface SafeSerializeOptions {
  /** JSON size cap in bytes; above it the value becomes a truncation marker. */
  maxBytes?: number;
  /** Nesting depth above which a value becomes "[MaxDepth]". */
  maxDepth?: number;
}

export interface SafeSerializeResult {
  /** JSON-safe value, or the truncation marker. `undefined` when the input was undefined. */
  value: unknown;
  truncated: boolean;
  /** UTF-8 size of the serialized JSON before truncation. */
  bytes: number;
}

export interface TruncationMarker {
  __lensTruncated: true;
  bytes: number;
}

export const DEFAULT_MAX_BYTES = 1024 * 1024;
export const DEFAULT_MAX_DEPTH = 20;

function byteLength(json: string): number {
  if (typeof TextEncoder !== "undefined") return new TextEncoder().encode(json).length;
  let bytes = 0;
  for (let i = 0; i < json.length; i++) {
    const code = json.charCodeAt(i);
    if (code < 0x80) bytes += 1;
    else if (code < 0x800) bytes += 2;
    else if (code >= 0xd800 && code <= 0xdbff) {
      bytes += 4;
      i++;
    } else bytes += 3;
  }
  return bytes;
}

/**
 * Converts any value into something JSON.stringify can handle without throwing:
 * BigInt to string, Date to ISO string, functions to "[Function]", circular
 * references to "[Circular]", Map/Set to arrays, Error to {name, message, stack}.
 * `undefined` stays undefined so JSON.stringify omits the property.
 */
export function toSafeJson(value: unknown, maxDepth = DEFAULT_MAX_DEPTH): unknown {
  return walk(value, 0, [], maxDepth);
}

function walk(value: unknown, depth: number, ancestors: object[], maxDepth: number): unknown {
  switch (typeof value) {
    case "undefined":
    case "boolean":
    case "string":
      return value;
    case "number":
      return Number.isFinite(value) ? value : String(value);
    case "bigint":
      return value.toString();
    case "function":
      return "[Function]";
    case "symbol":
      return value.toString();
    case "object":
      break;
    default:
      return String(value);
  }
  if (value === null) return null;
  if (value instanceof Date) {
    return Number.isNaN(value.getTime()) ? "Invalid Date" : value.toISOString();
  }
  if (value instanceof Error) {
    return { name: value.name, message: value.message, stack: value.stack };
  }
  if (ancestors.includes(value)) return "[Circular]";
  if (depth >= maxDepth) return "[MaxDepth]";

  const next = [...ancestors, value];
  if (Array.isArray(value)) {
    return value.map((item) => {
      const out = walk(item, depth + 1, next, maxDepth);
      return out === undefined ? null : out;
    });
  }
  if (value instanceof Map) {
    return Array.from(value.entries()).map(([k, v]) => [
      walk(k, depth + 1, next, maxDepth) ?? null,
      walk(v, depth + 1, next, maxDepth) ?? null,
    ]);
  }
  if (value instanceof Set) {
    return Array.from(value.values()).map((v) => walk(v, depth + 1, next, maxDepth) ?? null);
  }
  if (ArrayBuffer.isView(value) && !(value instanceof DataView)) {
    return Array.from(value as unknown as ArrayLike<number>);
  }

  const out: Record<string, unknown> = {};
  for (const key of Object.keys(value)) {
    const item = walk((value as Record<string, unknown>)[key], depth + 1, next, maxDepth);
    if (item !== undefined) out[key] = item;
  }
  return out;
}

/**
 * Serializes a value safely and enforces a size cap. When the JSON exceeds
 * `maxBytes` the value is replaced with `{ "__lensTruncated": true, "bytes": n }`.
 */
export function safeSerialize(
  value: unknown,
  options: SafeSerializeOptions = {},
): SafeSerializeResult {
  const maxBytes = options.maxBytes ?? DEFAULT_MAX_BYTES;
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;
  if (value === undefined) return { value: undefined, truncated: false, bytes: 0 };
  const safe = toSafeJson(value, maxDepth);
  const json = JSON.stringify(safe);
  if (json === undefined) return { value: undefined, truncated: false, bytes: 0 };
  const bytes = byteLength(json);
  if (bytes > maxBytes) {
    const marker: TruncationMarker = { __lensTruncated: true, bytes };
    return { value: marker, truncated: true, bytes };
  }
  return { value: safe, truncated: false, bytes };
}
