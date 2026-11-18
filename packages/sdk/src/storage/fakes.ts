// In-memory fakes of the three storage APIs, for tests. They follow the
// surfaces the adapter depends on (types.ts), not the full libraries.
import type { AsyncStorageLike, MMKVLike, MMKVValue, SecureStoreLike } from "./types";

export class FakeAsyncStorage implements AsyncStorageLike {
  readonly data = new Map<string, string>();
  getAllKeysCalls = 0;
  getItemCalls = 0;

  constructor(seed: Record<string, string> = {}) {
    for (const [k, v] of Object.entries(seed)) this.data.set(k, v);
  }
  async getAllKeys(): Promise<readonly string[]> {
    this.getAllKeysCalls += 1;
    return [...this.data.keys()];
  }
  async getItem(key: string): Promise<string | null> {
    this.getItemCalls += 1;
    return this.data.get(key) ?? null;
  }
  async setItem(key: string, value: string): Promise<void> {
    this.data.set(key, value);
  }
  async removeItem(key: string): Promise<void> {
    this.data.delete(key);
  }
  async multiRemove(keys: readonly string[]): Promise<void> {
    for (const key of keys) this.data.delete(key);
  }
  async clear(): Promise<void> {
    this.data.clear();
  }
}

/**
 * Typed entries like react-native-mmkv v3: each getter answers only for an
 * entry of its own type and returns undefined otherwise.
 */
export class FakeMMKV implements MMKVLike {
  readonly data = new Map<string, MMKVValue>();
  readonly listeners = new Set<(key: string) => void>();

  constructor(seed: Record<string, MMKVValue> = {}) {
    for (const [k, v] of Object.entries(seed)) this.data.set(k, v);
  }
  private notify(key: string): void {
    for (const listener of [...this.listeners]) listener(key);
  }
  getAllKeys(): string[] {
    return [...this.data.keys()];
  }
  getString(key: string): string | undefined {
    const v = this.data.get(key);
    return typeof v === "string" ? v : undefined;
  }
  getNumber(key: string): number | undefined {
    const v = this.data.get(key);
    return typeof v === "number" ? v : undefined;
  }
  getBoolean(key: string): boolean | undefined {
    const v = this.data.get(key);
    return typeof v === "boolean" ? v : undefined;
  }
  getBuffer(key: string): ArrayBuffer | Uint8Array | undefined {
    const v = this.data.get(key);
    return v instanceof Uint8Array || v instanceof ArrayBuffer ? v : undefined;
  }
  contains(key: string): boolean {
    return this.data.has(key);
  }
  set(key: string, value: MMKVValue): void {
    this.data.set(key, value);
    this.notify(key);
  }
  delete(key: string): void {
    this.data.delete(key);
    this.notify(key);
  }
  clearAll(): void {
    const keys = [...this.data.keys()];
    this.data.clear();
    for (const key of keys) this.notify(key);
  }
  addOnValueChangedListener(listener: (key: string) => void): { remove(): void } {
    this.listeners.add(listener);
    return { remove: () => this.listeners.delete(listener) };
  }
}

export class FakeSecureStore implements SecureStoreLike {
  readonly data = new Map<string, string>();

  constructor(seed: Record<string, string> = {}) {
    for (const [k, v] of Object.entries(seed)) this.data.set(k, v);
  }
  async getItemAsync(key: string): Promise<string | null> {
    return this.data.get(key) ?? null;
  }
  async setItemAsync(key: string, value: string): Promise<void> {
    this.data.set(key, value);
  }
  async deleteItemAsync(key: string): Promise<void> {
    this.data.delete(key);
  }
}
