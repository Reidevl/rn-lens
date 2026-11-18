import type { MCPInfo } from "@/backend";

/** Quotes s as one literal POSIX shell word. */
function shellQuote(s: string): string {
  return `'${s.replaceAll("'", `'\\''`)}'`;
}

/** The Claude Code command that adds this Lens as an MCP server. */
export function mcpCommand({ url, token }: MCPInfo): string {
  const base = `claude mcp add --transport http lens ${url}`;
  return token ? `${base} --header ${shellQuote(`Authorization: Bearer ${token}`)}` : base;
}
