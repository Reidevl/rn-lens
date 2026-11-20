import { distance, jwtStatus, type DecodedJwt, type JwtStatus } from "@/shared/lib/jwt";
import { cn } from "@/shared/lib/cn";
import { useNow } from "@/shared/lib/useNow";

import { JsonTree } from "./JsonTree";

const STATUS: Record<JwtStatus, { label: string; className: string }> = {
  valid: { label: "Valid", className: "text-accent" },
  expired: { label: "Expired", className: "text-destructive" },
  "not-yet-valid": { label: "Not valid yet", className: "text-warning" },
  "no-expiry": { label: "No expiry", className: "text-muted-foreground" },
};

function When({ label, ms, now }: { label: string; ms: number | undefined; now: number }) {
  if (ms === undefined) return null;
  return (
    <div className="contents">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="select-text">
        {new Date(ms).toLocaleString()}{" "}
        <span className="text-muted-foreground">({distance(ms, now)})</span>
      </dd>
    </div>
  );
}

/**
 * A decoded JWT: where it stands now, its time claims, header and payload.
 * The signature is not verified: Lens has no key, and the point is to read
 * what the app holds or sends.
 */
export function JwtView({ jwt }: { jwt: DecodedJwt }) {
  const now = useNow();
  const status = STATUS[jwtStatus(jwt, now)];
  return (
    <div className="grid gap-3 text-sm">
      <dl className="grid grid-cols-[minmax(6rem,auto)_1fr] gap-x-3 gap-y-0.5 font-mono">
        <dt className="text-muted-foreground">Status</dt>
        <dd className={cn("font-medium", status.className)} data-testid="jwt-status">
          {status.label}
          {jwt.expiresAt !== undefined && jwtStatus(jwt, now) !== "not-yet-valid" ? (
            <span className="font-normal text-muted-foreground">
              {" "}
              · {now >= jwt.expiresAt ? "expired" : "expires"} {distance(jwt.expiresAt, now)}
            </span>
          ) : null}
        </dd>
        <When label="Issued" ms={jwt.issuedAt} now={now} />
        <When label="Not before" ms={jwt.notBefore} now={now} />
        <When label="Expires" ms={jwt.expiresAt} now={now} />
      </dl>
      <section>
        <h3 className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          Header
        </h3>
        <JsonTree value={jwt.header} label="JWT header" />
      </section>
      <section>
        <h3 className="mb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          Payload
        </h3>
        <JsonTree value={jwt.payload} label="JWT payload" />
      </section>
      <p className="text-xs text-muted-foreground">
        Signature not verified: Lens decodes the token, it does not check it.
      </p>
    </div>
  );
}
