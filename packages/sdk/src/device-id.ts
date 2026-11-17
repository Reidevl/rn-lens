import { loadAsyncStorage as loadAsyncStorageModule } from "./optional-require.cjs";

const STORAGE_KEY = "@rnlens/device-id";

interface AsyncStorageLike {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
}

let inMemoryId: string | undefined;

function randomId(): string {
  const cryptoObj = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto;
  if (typeof cryptoObj?.randomUUID === "function") return cryptoObj.randomUUID();
  let out = "";
  for (let i = 0; i < 32; i++) {
    if (i === 8 || i === 12 || i === 16 || i === 20) out += "-";
    out += Math.floor(Math.random() * 16).toString(16);
  }
  return out;
}

function loadAsyncStorage(): AsyncStorageLike | undefined {
  const mod = loadAsyncStorageModule<{ default?: AsyncStorageLike } & Partial<AsyncStorageLike>>();
  const storage = mod?.default ?? mod;
  if (storage && typeof storage.getItem === "function" && typeof storage.setItem === "function") {
    return storage as AsyncStorageLike;
  }
  return undefined;
}

/**
 * Returns a device id that is stable per install when AsyncStorage is present,
 * and stable per process otherwise.
 */
export async function getDeviceId(): Promise<string> {
  if (inMemoryId !== undefined) return inMemoryId;
  const storage = loadAsyncStorage();
  if (storage) {
    try {
      const stored = await storage.getItem(STORAGE_KEY);
      if (stored) {
        inMemoryId = stored;
        return stored;
      }
      const fresh = randomId();
      await storage.setItem(STORAGE_KEY, fresh);
      inMemoryId = fresh;
      return fresh;
    } catch {
      // fall through to the in-memory id
    }
  }
  inMemoryId = randomId();
  return inMemoryId;
}

/** Test hook: forgets the in-memory id so the next call derives a new one. */
export function resetDeviceIdForTests(): void {
  inMemoryId = undefined;
}
