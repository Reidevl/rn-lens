import type { MCPInfo } from "@/backend";

/** The Claude Code command that adds this Lens as an MCP server. */
export function mcpCommand({ url, token }: MCPInfo): string {
  const base = `claude mcp add --transport http lens ${url}`;
  return token ? `${base} --header "Authorization: Bearer ${token}"` : base;
}
