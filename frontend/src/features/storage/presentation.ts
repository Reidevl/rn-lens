import type { StorageKind, StorageValue } from "@/backend";

export const KIND_LABEL: Record<StorageKind, string> = {
  "async-storage": "AsyncStorage",
  mmkv: "MMKV",
  "secure-store": "SecureStore",
};

/** The value's bytes: base64-decoded for bytes, UTF-8 of the text otherwise. */
export function bytesOf(v: Pick<StorageValue, "type" | "value">): Uint8Array {
  if (v.type === "bytes") {
    try {
      return Uint8Array.from(atob(v.value), (c) => c.charCodeAt(0));
    } catch {
      return new Uint8Array();
    }
  }
  return new TextEncoder().encode(v.value);
}

/** Size in bytes, using the size the SDK reported for truncated values. */
export function valueSize(v: StorageValue): number {
  return v.truncated ? (v.size ?? 0) : bytesOf(v).length;
}

/** Classic hex dump: offset, 16 bytes in hex, printable ASCII. */
export function hexDump(v: Pick<StorageValue, "type" | "value">): string {
  const bytes = bytesOf(v);
  const lines: string[] = [];
  for (let off = 0; off < bytes.length; off += 16) {
    const chunk = bytes.subarray(off, off + 16);
    const hex = [...chunk].map((b) => b.toString(16).padStart(2, "0")).join(" ");
    const ascii = [...chunk]
      .map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : "."))
      .join("");
    lines.push(`${off.toString(16).padStart(8, "0")}  ${hex.padEnd(47)}  |${ascii}|`);
  }
  return lines.join("\n");
}

export function toBase64(v: Pick<StorageValue, "type" | "value">): string {
  if (v.type === "bytes") return v.value;
  let binary = "";
  for (const b of bytesOf(v)) binary += String.fromCharCode(b);
  return btoa(binary);
}

/** JSON objects and arrays held in a string value. */
export function jsonOf(
  v: Pick<StorageValue, "type" | "value">,
): { ok: true; value: unknown } | { ok: false } {
  if (v.type !== "string") return { ok: false };
  const t = v.value.trim();
  if (!t.startsWith("{") && !t.startsWith("[")) return { ok: false };
  try {
    return { ok: true, value: JSON.parse(t) as unknown };
  } catch {
    return { ok: false };
  }
}
