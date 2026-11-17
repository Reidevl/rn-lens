import { create } from "zustand";

import { idleSession, type Backend, type SessionSnapshot, type Target } from "@/backend";

interface TargetsState {
  targets: Target[];
  session: SessionSnapshot;
  /** Last error from an action, shown inline until the next action. */
  error: string | null;
  busy: boolean;
  setTargets: (targets: Target[]) => void;
  setSession: (session: SessionSnapshot) => void;
  setError: (error: string | null) => void;
  setBusy: (busy: boolean) => void;
}

export const useTargetsStore = create<TargetsState>((set) => ({
  targets: [],
  session: idleSession,
  error: null,
  busy: false,
  setTargets: (targets) => set({ targets }),
  setSession: (session) => set({ session }),
  setError: (error) => set({ error }),
  setBusy: (busy) => set({ busy }),
}));

/** Pulls both targets and the session from the backend into the store. */
export async function pullTargets(backend: Backend): Promise<void> {
  const [targets, session] = await Promise.all([backend.targets.list(), backend.targets.session()]);
  useTargetsStore.getState().setTargets(targets);
  useTargetsStore.getState().setSession(session);
}

export async function pullSession(backend: Backend): Promise<void> {
  useTargetsStore.getState().setSession(await backend.targets.session());
}

/** Runs an action, surfacing its error in the store. */
export async function runAction(backend: Backend, action: () => Promise<void>): Promise<void> {
  const store = useTargetsStore.getState();
  store.setBusy(true);
  store.setError(null);
  try {
    await action();
  } catch (err) {
    store.setError(err instanceof Error ? err.message : String(err));
  } finally {
    useTargetsStore.getState().setBusy(false);
    await pullSession(backend).catch(() => {});
  }
}
