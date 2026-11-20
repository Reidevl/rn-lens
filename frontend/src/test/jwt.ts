/** Builds a compact JWT from JSON parts for tests; the signature is opaque text. */
export function makeJwt(header: object, payload: object, signature = "c2ln"): string {
  const enc = (o: object) =>
    btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(o))))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  return `${enc(header)}.${enc(payload)}.${signature}`;
}
