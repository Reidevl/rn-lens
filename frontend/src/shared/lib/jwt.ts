/** A decoded JSON Web Token. The signature is kept as text, never verified. */
export interface DecodedJwt {
  /** The token without any "Bearer " prefix. */
  token: string;
  header: Record<string, unknown>;
  payload: Record<string, unknown>;
  signature: string;
  /** Registered time claims in milliseconds, when present and numeric. */
  issuedAt?: number;
  notBefore?: number;
  expiresAt?: number;
}

const SHAPE = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*$/;

function base64UrlJson(part: string): Record<string, unknown> | undefined {
  try {
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const bytes = Uint8Array.from(atob(padded), (c) => c.charCodeAt(0));
    const value: unknown = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
    return value !== null && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}

const seconds = (v: unknown): number | undefined =>
  typeof v === "number" && Number.isFinite(v) ? v * 1000 : undefined;

/**
 * Decodes `text` when it is a JWS compact token (header.payload.signature),
 * optionally prefixed with "Bearer ". Returns undefined for anything else:
 * the header must be a JSON object with an `alg`, which keeps dotted strings
 * such as host names or versions from passing as tokens.
 */
export function decodeJwt(text: string): DecodedJwt | undefined {
  const token = text.trim().replace(/^bearer\s+/i, "");
  if (!SHAPE.test(token)) return undefined;
  const [h, p, signature] = token.split(".");
  const header = base64UrlJson(h);
  if (!header || typeof header.alg !== "string") return undefined;
  const payload = base64UrlJson(p);
  if (!payload) return undefined;
  return {
    token,
    header,
    payload,
    signature,
    issuedAt: seconds(payload.iat),
    notBefore: seconds(payload.nbf),
    expiresAt: seconds(payload.exp),
  };
}

/** "in 5m", "3h ago": a short distance between `ms` and `now`. */
export function distance(ms: number, now: number): string {
  const s = Math.round(Math.abs(ms - now) / 1000);
  const amount =
    s < 60
      ? `${s}s`
      : s < 3600
        ? `${Math.floor(s / 60)}m`
        : s < 86_400
          ? `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
          : `${Math.floor(s / 86_400)}d ${Math.floor((s % 86_400) / 3600)}h`;
  return ms >= now ? `in ${amount}` : `${amount} ago`;
}

export type JwtStatus = "valid" | "expired" | "not-yet-valid" | "no-expiry";

/** Where the token stands at `now`, from its exp and nbf claims. */
export function jwtStatus(jwt: DecodedJwt, now: number): JwtStatus {
  if (jwt.notBefore !== undefined && now < jwt.notBefore) return "not-yet-valid";
  if (jwt.expiresAt === undefined) return "no-expiry";
  return now >= jwt.expiresAt ? "expired" : "valid";
}
