import { create } from "zustand";

import type { PanelId } from "./panels";

interface ShellState {
  activePanel: PanelId;
  setActivePanel: (panel: PanelId) => void;
}

export const useShellStore = create<ShellState>((set) => ({
  activePanel: "targets",
  setActivePanel: (activePanel) => set({ activePanel }),
}));
