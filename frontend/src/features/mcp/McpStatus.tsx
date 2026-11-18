import { Bot } from "lucide-react";
import { useEffect, useState } from "react";

import { useBackend, type MCPInfo } from "@/backend";

import { mcpCommand } from "./command";

/** Status bar item: copies the command that connects an AI client to Lens. */
export function McpStatus() {
  const backend = useBackend();
  const [info, setInfo] = useState<MCPInfo | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    backend.app
      .mcp()
      .then((value) => {
        if (!cancelled) setInfo(value);
      })
      .catch(() => {
        // Without the endpoint the item stays hidden.
      });
    return () => {
      cancelled = true;
    };
  }, [backend]);

  useEffect(() => {
    if (!copied) return;
    const id = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(id);
  }, [copied]);

  if (!info?.url) return null;
  return (
    <button
      type="button"
      aria-label="Copy the command that connects Claude to Lens (MCP)"
      title={`MCP at ${info.url}. Click to copy: ${mcpCommand(info)}`}
      onClick={() =>
        void backend.clipboard
          .writeText(mcpCommand(info))
          .then(() => setCopied(true))
          .catch(() => setCopied(false))
      }
      className="flex h-6 cursor-pointer items-center gap-1 rounded-md px-1.5 text-xs text-muted-foreground transition-colors duration-150 hover:bg-muted hover:text-foreground"
    >
      <Bot aria-hidden="true" className="size-3.5" />
      {copied ? "Copied" : "MCP"}
    </button>
  );
}
