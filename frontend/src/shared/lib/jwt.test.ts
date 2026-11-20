import { describe, expect, it } from "vitest";

import { makeJwt } from "@/test/jwt";

import { decodeJwt, distance, jwtStatus } from "./jwt";

describe("decodeJwt", () => {
  const token = makeJwt(
    { alg: "HS256", typ: "JWT" },
    { sub: "42", name: "Zoë", iat: 1_700_000_000, nbf: 1_700_000_000, exp: 1_700_003_600 },
  );

  it("decodes the header, payload, signature and time claims", () => {
    const jwt = decodeJwt(token);
    expect(jwt).toEqual({
      token,
      header: { alg: "HS256", typ: "JWT" },
      payload: {
        sub: "42",
        name: "Zoë",
        iat: 1_700_000_000,
        nbf: 1_700_000_000,
        exp: 1_700_003_600,
      },
      signature: "c2ln",
      issuedAt: 1_700_000_000_000,
      notBefore: 1_700_000_000_000,
      expiresAt: 1_700_003_600_000,
    });
  });

  it("accepts a Bearer prefix and surrounding spaces", () => {
    expect(decodeJwt(`  Bearer ${token} `)?.token).toBe(token);
    expect(decodeJwt(`bearer ${token}`)?.token).toBe(token);
  });

  it("rejects strings that only look dotted", () => {
    expect(decodeJwt("api.example.com")).toBeUndefined();
    expect(decodeJwt("1.2.3")).toBeUndefined();
    expect(decodeJwt("not a token")).toBeUndefined();
    expect(decodeJwt("")).toBeUndefined();
    // Valid base64url JSON, but the header has no alg.
    expect(decodeJwt(makeJwt({ typ: "JWT" }, { sub: "1" }))).toBeUndefined();
    // A JSON array payload is not a claims set.
    const [h] = token.split(".");
    expect(decodeJwt(`${h}.${btoa("[1]").replace(/=+$/, "")}.x`)).toBeUndefined();
  });

  it("leaves time claims out when they are missing or not numbers", () => {
    const jwt = decodeJwt(makeJwt({ alg: "none" }, { exp: "soon" }, ""));
    expect(jwt?.expiresAt).toBeUndefined();
    expect(jwt?.issuedAt).toBeUndefined();
    expect(jwt?.signature).toBe("");
  });
});

describe("jwtStatus and distance", () => {
  const jwt = decodeJwt(makeJwt({ alg: "HS256" }, { nbf: 1000, exp: 2000 }))!;

  it("reports not-yet-valid, valid and expired from nbf and exp", () => {
    expect(jwtStatus(jwt, 999_000)).toBe("not-yet-valid");
    expect(jwtStatus(jwt, 1_500_000)).toBe("valid");
    expect(jwtStatus(jwt, 2_000_000)).toBe("expired");
    expect(jwtStatus(decodeJwt(makeJwt({ alg: "HS256" }, {}))!, 0)).toBe("no-expiry");
  });

  it("formats future and past distances", () => {
    expect(distance(30_000, 0)).toBe("in 30s");
    expect(distance(0, 5 * 60_000)).toBe("5m ago");
    expect(distance(2 * 3600_000 + 15 * 60_000, 0)).toBe("in 2h 15m");
    expect(distance(0, 3 * 86_400_000 + 4 * 3600_000)).toBe("3d 4h ago");
  });
});
