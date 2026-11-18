// Base64 without Buffer, atob or btoa, which React Native runtimes do not all
// provide for binary data.
const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
const LOOKUP = new Map<string, number>([...ALPHABET].map((char, index) => [char, index]));

export function encodeBase64(bytes: Uint8Array): string {
  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i];
    const b = i + 1 < bytes.length ? bytes[i + 1] : 0;
    const c = i + 2 < bytes.length ? bytes[i + 2] : 0;
    const triple = (a << 16) | (b << 8) | c;
    out += ALPHABET[(triple >> 18) & 63] + ALPHABET[(triple >> 12) & 63];
    out += i + 1 < bytes.length ? ALPHABET[(triple >> 6) & 63] : "=";
    out += i + 2 < bytes.length ? ALPHABET[triple & 63] : "=";
  }
  return out;
}

/** Decodes standard base64 (padding optional); returns undefined when invalid. */
export function decodeBase64(text: string): Uint8Array | undefined {
  const clean = text.replace(/=+$/, "");
  if (clean.length % 4 === 1) return undefined;
  const out = new Uint8Array(Math.floor((clean.length * 3) / 4));
  let buffer = 0;
  let bits = 0;
  let index = 0;
  for (const char of clean) {
    const value = LOOKUP.get(char);
    if (value === undefined) return undefined;
    buffer = (buffer << 6) | value;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      out[index++] = (buffer >> bits) & 0xff;
    }
  }
  return out;
}
