import {
  Activity,
  Bot,
  Database,
  type LucideIcon,
  Network,
  Puzzle,
  RefreshCw,
  Smartphone,
  SquareTerminal,
  Workflow,
} from "lucide-react";

export type PanelId =
  | "targets"
  | "console"
  | "network"
  | "queries"
  | "mutations"
  | "storage"
  | "performance"
  | "plugins"
  | "mcp";

export interface PanelDefinition {
  id: PanelId;
  label: string;
  icon: LucideIcon;
  /** Milestone that delivers the panel; shown while it is a placeholder. */
  milestone: string;
  /** "bottom" entries are about Lens itself and sit apart from the app's panels. */
  placement?: "bottom";
}

/** Left-rail entries, in display order. */
export const PANELS: readonly PanelDefinition[] = [
  { id: "targets", label: "Targets", icon: Smartphone, milestone: "M1" },
  { id: "console", label: "Console", icon: SquareTerminal, milestone: "M2" },
  { id: "network", label: "Network", icon: Network, milestone: "M4" },
  { id: "queries", label: "Queries", icon: RefreshCw, milestone: "M3" },
  { id: "mutations", label: "Mutations", icon: Workflow, milestone: "M3" },
  { id: "storage", label: "Storage", icon: Database, milestone: "M5" },
  { id: "performance", label: "Performance", icon: Activity, milestone: "M6" },
  { id: "plugins", label: "Plugins", icon: Puzzle, milestone: "M7" },
  { id: "mcp", label: "MCP", icon: Bot, milestone: "M8", placement: "bottom" },
];
