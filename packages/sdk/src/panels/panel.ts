import type { PluginBlock } from "@rnlens/protocol";

/** A click in Lens on one of the panel's actions. */
export interface LensPanelAction {
  blockId: string;
  actionId: string;
  /** The row, for a table row action. */
  rowId?: string;
}

export interface LensPanelDefinition {
  /** Stable id: lowercase letters, digits, ".", "_" or "-" (at most 64). */
  id: string;
  /** Shown as the panel's tab in Lens. */
  name: string;
  description?: string;
  version?: string;
  /** Describes the whole panel with Lens building blocks. */
  render(): PluginBlock[] | Promise<PluginBlock[]>;
  /**
   * Runs when the user clicks an action. Return a short message to show in
   * Lens; throw to report a failure. The panel re-renders afterwards.
   */
  onAction?(action: LensPanelAction): void | string | Promise<void | string>;
}

/** A panel Lens draws for the app; pass it to `useLens({ panels })`. */
export interface LensPanel extends Readonly<LensPanelDefinition> {
  /** Sends the panel again; call it when the data it shows changed. */
  refresh(): void;
  /** @internal Called by the client on refresh; returns an unsubscribe function. */
  subscribe(listener: () => void): () => void;
}

const PANEL_ID = /^[a-z0-9][a-z0-9._-]{0,63}$/;

/** Creates a declarative panel: Lens draws its blocks, no UI code runs in Lens. */
export function createLensPanel(definition: LensPanelDefinition): LensPanel {
  if (!PANEL_ID.test(definition.id)) {
    throw new Error(
      `@rnlens/sdk: invalid panel id "${definition.id}": use lowercase letters, digits, ".", "_" or "-"`,
    );
  }
  if (!definition.name || definition.name.length > 64) {
    throw new Error(`@rnlens/sdk: panel "${definition.id}" needs a name of 1 to 64 characters`);
  }
  const listeners = new Set<() => void>();
  return {
    ...definition,
    refresh() {
      for (const listener of listeners) listener();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}
