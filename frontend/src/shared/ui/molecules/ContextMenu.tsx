import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";

import { cn } from "@/shared/lib/cn";

export interface ContextMenuItem {
  label: string;
  onSelect: () => void;
  disabled?: boolean;
}

const EDGE_PX = 4;

/**
 * A right-click menu at viewport point (x, y). It takes focus, moves with
 * the arrow keys, and closes on Escape, an outside press, scroll, resize or
 * after an item runs.
 */
export function ContextMenu({
  x,
  y,
  items,
  label,
  onClose,
}: {
  x: number;
  y: number;
  items: ContextMenuItem[];
  /** Accessible name of the menu. */
  label: string;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });

  // Keep the menu inside the window, then focus its first enabled item.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const { width, height } = el.getBoundingClientRect();
    setPos({
      left: Math.max(EDGE_PX, Math.min(x, window.innerWidth - width - EDGE_PX)),
      top: Math.max(EDGE_PX, Math.min(y, window.innerHeight - height - EDGE_PX)),
    });
    el.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();
  }, [x, y]);

  useEffect(() => {
    const onPointerDown = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    window.addEventListener("pointerdown", onPointerDown, true);
    window.addEventListener("scroll", onClose, true);
    window.addEventListener("resize", onClose);
    window.addEventListener("blur", onClose);
    return () => {
      window.removeEventListener("pointerdown", onPointerDown, true);
      window.removeEventListener("scroll", onClose, true);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("blur", onClose);
    };
  }, [onClose]);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
      return;
    }
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const buttons = [
      ...(ref.current?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)") ?? []),
    ];
    if (buttons.length === 0) return;
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const step = e.key === "ArrowDown" ? 1 : -1;
    buttons[(at + step + buttons.length) % buttons.length].focus();
  };

  return (
    <div
      ref={ref}
      role="menu"
      aria-label={label}
      onKeyDown={onKeyDown}
      onContextMenu={(e) => e.preventDefault()}
      style={{ left: pos.left, top: pos.top }}
      className="fixed z-50 min-w-40 rounded-md border border-border bg-card p-1 text-sm shadow-lg"
    >
      {items.map((item) => (
        <button
          key={item.label}
          type="button"
          role="menuitem"
          disabled={item.disabled}
          onClick={() => {
            onClose();
            item.onSelect();
          }}
          className={cn(
            "flex w-full cursor-pointer items-center rounded px-2 py-1 text-left",
            "hover:bg-muted focus-visible:bg-muted focus-visible:outline-none",
            "disabled:cursor-default disabled:opacity-50 disabled:hover:bg-transparent",
          )}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
