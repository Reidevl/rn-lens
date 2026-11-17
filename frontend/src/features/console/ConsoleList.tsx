import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown } from "lucide-react";
import { useEffect, useRef } from "react";

import type { ConsoleEntry } from "@/backend";
import { Button } from "@/shared/ui/atoms/Button";

import { ConsoleRow } from "./ConsoleRow";
import { useConsoleStore } from "./consoleStore";

const FOLLOW_THRESHOLD_PX = 8;

/** Virtualized log that follows the tail until the user scrolls up. */
export function ConsoleList({ entries }: { entries: ConsoleEntry[] }) {
  const parentRef = useRef<HTMLDivElement>(null);
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

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={parentRef}
        role="log"
        aria-label="Console output"
        onScroll={onScroll}
        className="h-full overflow-auto"
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
    </div>
  );
}
