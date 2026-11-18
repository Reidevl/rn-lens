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
  text = "Copy",
}: {
  label: string;
  copy: () => Promise<void>;
  disabled?: boolean;
  /** Visible text while idle; defaults to "Copy". */
  text?: string;
}) {
  const [status, setStatus] = useState<Status>("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clearTimeout(timer.current);
    };
  }, []);

  const onClick = () => {
    clearTimeout(timer.current);
    const settle = (next: Status) => {
      if (!mounted.current) return;
      setStatus(next);
      timer.current = setTimeout(() => setStatus("idle"), RESET_MS);
    };
    copy().then(
      () => settle("copied"),
      () => settle("failed"),
    );
  };

  return (
    <Button size="sm" aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      {status === "copied" ? (
        <Check aria-hidden="true" className="size-3.5 text-accent" />
      ) : (
        <Copy aria-hidden="true" className="size-3.5" />
      )}
      <span className={status === "failed" ? "text-destructive" : undefined}>
        {status === "copied" ? "Copied" : status === "failed" ? "Copy failed" : text}
      </span>
    </Button>
  );
}
