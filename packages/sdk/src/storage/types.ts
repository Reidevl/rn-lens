// Structural surfaces of the storage libraries the adapter reads and writes.
// The SDK never imports these libraries: the app passes its own instances.

/** `@react-native-async-storage/async-storage` default export. */
export interface AsyncStorageLike {
  getAllKeys(): Promise<readonly string[]>;
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  removeItem(key: string): Promise<void>;
  multiRemove?(keys: readonly string[]): Promise<void>;
  clear?(): Promise<void>;
}

export type MMKVValue = string | number | boolean | ArrayBuffer | Uint8Array;

/** A `react-native-mmkv` v3 `MMKV` instance. */
export interface MMKVLike {
  getAllKeys(): string[];
  getString(key: string): string | undefined;
  getNumber(key: string): number | undefined;
  getBoolean(key: string): boolean | undefined;
  getBuffer(key: string): ArrayBuffer | Uint8Array | undefined;
  set(key: string, value: MMKVValue): void;
  delete(key: string): void;
  clearAll(): void;
  contains(key: string): boolean;
  addOnValueChangedListener(listener: (key: string) => void): { remove(): void };
}

/** The `expo-secure-store` module. */
export interface SecureStoreLike {
  getItemAsync(key: string): Promise<string | null>;
  setItemAsync(key: string, value: string): Promise<void>;
  deleteItemAsync(key: string): Promise<void>;
}

export interface LensStorageOptions {
  /** AsyncStorage instance; listed as store "async-storage". */
  asyncStorage?: AsyncStorageLike;
  /** MMKV instances by name; each is listed as store "mmkv:<name>". */
  mmkv?: Record<string, MMKVLike>;
  /** SecureStore cannot enumerate keys: only `keys` are listed and editable. */
  secureStore?: { module: SecureStoreLike; keys: string[] };
  /** Keys hidden from Lens: a string matches exactly, a RegExp is tested. */
  exclude?: Array<string | RegExp>;
}
