import { Check, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/shared/ui/atoms/Button";

type Status = "idle" | "copied" | "failed";

const RESET_MS = 1_500;

/**
 * Copies text produced by `copy` and confirms in place ("Copied" or
 * "Copy failed") for a moment. `label` is the accessible name and stays the
 * same in every state, so tests and screen readers can keep finding it.
 */
export function CopyButton({
  label,
  copy,
  disabled,
}: {
  label: string;
  copy: () => Promise<void>;
  disabled?: boolean;
}) {
  const [status, setStatus] = useState<Status>("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  const onClick = () => {
    clearTimeout(timer.current);
    copy()
      .then(
        () => setStatus("copied"),
        () => setStatus("failed"),
      )
      .finally(() => {
        timer.current = setTimeout(() => setStatus("idle"), RESET_MS);
      });
  };

  return (
    <Button size="sm" aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      {status === "copied" ? (
        <Check aria-hidden="true" className="size-3.5 text-accent" />
      ) : (
        <Copy aria-hidden="true" className="size-3.5" />
      )}
      <span className={status === "failed" ? "text-destructive" : undefined}>
        {status === "copied" ? "Copied" : status === "failed" ? "Copy failed" : "Copy"}
      </span>
    </Button>
  );
}
