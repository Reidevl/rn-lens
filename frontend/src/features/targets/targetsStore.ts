import { create } from "zustand";

import { idleSession, type Backend, type SessionSnapshot, type Target } from "@/backend";

/**
 * Orders overlapping pulls of one resource. Every pull takes the next `issued`
 * token when it starts; its result is applied only when no pull with a higher
 * token has been applied yet, so a slow older answer never overwrites a newer
 * one (for example a `connecting` answer landing after a terminal `closed`).
 */
export interface PullOrder {
  /** Token handed to the latest pull that started. */
  issued: number;
  /** Token of the latest pull whose result reached the store. */
  applied: number;
}

type PullKey = "targets" | "session";

const noPulls: PullOrder = { issued: 0, applied: 0 };

interface TargetsState {
  targets: Target[];
  session: SessionSnapshot;
  /** Last error from an action, shown inline until the next action. */
  error: string | null;
  busy: boolean;
  /** Ordering guards for the targets and session pulls. */
  pulls: Record<PullKey, PullOrder>;
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
  pulls: { targets: noPulls, session: noPulls },
  setTargets: (targets) => set({ targets }),
  setSession: (session) => set({ session }),
  setError: (error) => set({ error }),
  setBusy: (busy) => set({ busy }),
}));

/** Reserves the next token for a pull of `key`. */
function beginPull(key: PullKey): number {
  const { pulls } = useTargetsStore.getState();
  const token = pulls[key].issued + 1;
  useTargetsStore.setState({ pulls: { ...pulls, [key]: { ...pulls[key], issued: token } } });
  return token;
}

/**
 * Records `token` as applied and returns true, or returns false when a newer
 * pull was applied first and this result must be discarded.
 */
function commitPull(key: PullKey, token: number): boolean {
  const { pulls } = useTargetsStore.getState();
  if (token <= pulls[key].applied) return false;
  useTargetsStore.setState({ pulls: { ...pulls, [key]: { ...pulls[key], applied: token } } });
  return true;
}

/** Pulls both targets and the session from the backend into the store. */
export async function pullTargets(backend: Backend): Promise<void> {
  const targetsToken = beginPull("targets");
  const sessionToken = beginPull("session");
  const [targets, session] = await Promise.all([backend.targets.list(), backend.targets.session()]);
  const store = useTargetsStore.getState();
  if (commitPull("targets", targetsToken)) store.setTargets(targets);
  if (commitPull("session", sessionToken)) store.setSession(session);
}

export async function pullSession(backend: Backend): Promise<void> {
  const token = beginPull("session");
  const session = await backend.targets.session();
  if (commitPull("session", token)) useTargetsStore.getState().setSession(session);
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
