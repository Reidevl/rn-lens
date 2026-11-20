import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type MouseEvent } from "react";

import type { ConsoleEntry } from "@/backend";
import { Button } from "@/shared/ui/atoms/Button";
import { ContextMenu } from "@/shared/ui/molecules/ContextMenu";

import { ConsoleRow } from "./ConsoleRow";
import { useConsoleStore } from "./consoleStore";

const FOLLOW_THRESHOLD_PX = 8;

/** The selected text when the selection lies inside `root`, else "". */
function selectionWithin(root: HTMLElement | null): string {
  const sel = window.getSelection();
  if (!root || !sel || sel.rangeCount === 0) return "";
  return root.contains(sel.getRangeAt(0).commonAncestorContainer) ? sel.toString() : "";
}

interface MenuState {
  x: number;
  y: number;
  /** Text selected when the menu opened. */
  selection: string;
  /** The row under the pointer. */
  entry?: ConsoleEntry;
}

/**
 * Virtualized log that follows the tail until the user scrolls up. Text is
 * selectable; a right-click offers Copy (the selection) and Copy message.
 */
export function ConsoleList({
  entries,
  onCopy,
}: {
  entries: ConsoleEntry[];
  /** Puts text on the clipboard; without it there is no right-click menu. */
  onCopy?: (text: string) => Promise<void>;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  const [menu, setMenu] = useState<MenuState | null>(null);
  const closeMenu = useCallback(() => setMenu(null), []);
  const follow = useConsoleStore((s) => s.follow);
  const setFollow = useConsoleStore((s) => s.setFollow);

  // TanStack Virtual keeps its own mutable instance, which the React Compiler
  // cannot memoize; the component is small enough to skip compilation.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 26,
    overscan: 20,
    getItemKey: (index) => entries[index].seq,
  });

  useEffect(() => {
    if (follow && entries.length > 0) {
      virtualizer.scrollToIndex(entries.length - 1, { align: "end" });
    }
  }, [entries.length, follow, virtualizer]);

  const onScroll = () => {
    const el = parentRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= FOLLOW_THRESHOLD_PX;
    if (atBottom !== follow) setFollow(atBottom);
  };

  const onContextMenu = (e: MouseEvent<HTMLDivElement>) => {
    if (!onCopy) return;
    e.preventDefault();
    const row = (e.target as HTMLElement).closest<HTMLElement>("[data-index]");
    const index = row ? Number(row.dataset.index) : -1;
    setMenu({
      x: e.clientX,
      y: e.clientY,
      selection: selectionWithin(parentRef.current),
      entry: index >= 0 ? entries[index] : undefined,
    });
  };

  const copy = (text: string) => {
    void onCopy?.(text).catch(() => {});
  };

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={parentRef}
        role="log"
        aria-label="Console output"
        onScroll={onScroll}
        onContextMenu={onContextMenu}
        className="h-full overflow-auto select-text"
      >
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((item) => (
            <div
              key={item.key}
              data-index={item.index}
              ref={virtualizer.measureElement}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                transform: `translateY(${item.start}px)`,
              }}
            >
              <ConsoleRow entry={entries[item.index]} />
            </div>
          ))}
        </div>
      </div>
      {!follow && entries.length > 0 ? (
        <Button
          size="sm"
          variant="primary"
          className="absolute right-4 bottom-3 shadow-lg"
          onClick={() => {
            setFollow(true);
            virtualizer.scrollToIndex(entries.length - 1, { align: "end" });
          }}
        >
          <ArrowDown aria-hidden="true" className="size-3.5" />
          Jump to latest
        </Button>
      ) : null}
      {menu ? (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          label="Console actions"
          onClose={closeMenu}
          items={[
            {
              label: "Copy",
              disabled: menu.selection === "",
              onSelect: () => copy(menu.selection),
            },
            {
              label: "Copy message",
              disabled: !menu.entry,
              onSelect: () => menu.entry && copy(menu.entry.text),
            },
          ]}
        />
      ) : null}
    </div>
  );
}
