import AsyncStorage from "@react-native-async-storage/async-storage";
import type { LensStorageOptions, MMKVLike } from "@rnlens/sdk";
import * as SecureStore from "expo-secure-store";

/**
 * The stores the example exposes to Lens's Storage panel, with a few values
 * to look at. MMKV is native code that Expo Go does not ship: it loads only
 * in a development build (`npx expo run:ios`), and the example keeps working
 * without it.
 */
function loadMMKV(): MMKVLike | undefined {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { createMMKV } = require("react-native-mmkv") as typeof import("react-native-mmkv");
    return createMMKV({ id: "lens-example" }) as unknown as MMKVLike;
  } catch {
    return undefined;
  }
}

export const mmkv = loadMMKV();

// The Go end-to-end test reads AsyncStorage back through the debugger to check
// that an edit made in Lens reached the app. Development builds only.
if (__DEV__) {
  (globalThis as { __lensExample?: unknown }).__lensExample = { AsyncStorage, mmkv };
}

export const storage: LensStorageOptions = {
  asyncStorage: AsyncStorage,
  mmkv: mmkv ? { "lens-example": mmkv } : undefined,
  secureStore: { module: SecureStore, keys: ["token"] },
  // Keys matching these never reach Lens.
  exclude: [/^internal:/],
};

/** Seeds the stores once and counts launches, so every store has content. */
export async function seedStorage(): Promise<void> {
  if ((await AsyncStorage.getItem("theme")) === null) {
    await AsyncStorage.multiSet([
      ["theme", "dark"],
      ["profile", JSON.stringify({ name: "Ada", plan: "pro", tags: ["beta", "lens"] })],
      ["internal:cache-version", "3"],
    ]);
  }
  if ((await SecureStore.getItemAsync("token")) === null) {
    await SecureStore.setItemAsync("token", "example-token-not-a-secret");
  }
  if (mmkv) {
    mmkv.set("launches", (mmkv.getNumber("launches") ?? 0) + 1);
    mmkv.set("onboarded", true);
    mmkv.set("avatar", new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]).buffer);
  }
}
