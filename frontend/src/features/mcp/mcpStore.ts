import { create } from "zustand";

import type { Backend, McpCall, McpStatus, McpTool } from "@/backend";

interface McpState {
  status: McpStatus | null;
  tools: McpTool[];
  /** Recorded calls, oldest first. */
  calls: McpCall[];
  error: string | null;
  setError: (error: string | null) => void;
  reset: () => void;
}

const initial = {
  status: null as McpStatus | null,
  tools: [] as McpTool[],
  calls: [] as McpCall[],
  error: null as string | null,
};

export const useMcpStore = create<McpState>((set) => ({
  ...initial,
  setError: (error) => set({ error }),
  reset: () => set(initial),
}));

/**
 * Pulls the endpoint state and the whole activity log (at most 500 calls),
 * so a clear made anywhere shows up here too.
 */
export async function pullMcp(backend: Backend): Promise<void> {
  const [status, tools, calls] = await Promise.all([
    backend.mcp.status(),
    useMcpStore.getState().tools.length
      ? Promise.resolve(useMcpStore.getState().tools)
      : backend.mcp.tools(),
    backend.mcp.activity(0),
  ]);
  useMcpStore.setState({ status, tools, calls });
}
