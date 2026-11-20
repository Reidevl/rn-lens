# Storage

The Storage panel shows the key-value stores your app uses (AsyncStorage,
MMKV and SecureStore), lets you read, edit, add and delete keys, and exports
or imports a whole store. It needs `@rnlens/sdk` in the app.

## Quick path

1. Install the SDK and pass the stores you use to `useLens` (each is
   optional):

   ```tsx
   import AsyncStorage from "@react-native-async-storage/async-storage";
   import * as SecureStore from "expo-secure-store";
   import { createMMKV } from "react-native-mmkv";

   const mmkv = createMMKV();

   useLens({
     queryClient,
     storage: {
       asyncStorage: AsyncStorage,
       mmkv: { default: mmkv },
       secureStore: { module: SecureStore, keys: ["session.token"] },
       exclude: ["pin", /^auth\./],
     },
   });
   ```

2. Run the app in development and open **Storage** in Lens.

The SDK imports none of these libraries: it uses the instances you pass, so
it adds no dependency to your app. MMKV is native code; in an Expo app it
needs a development build (`npx expo run:ios`), not Expo Go.

## Stores

| Store        | What Lens lists                                       | How changes appear                                    |
| ------------ | ----------------------------------------------------- | ----------------------------------------------------- |
| AsyncStorage | every key, with sizes when the store has 500 or fewer | after each write from Lens, or with **Refresh**       |
| MMKV         | every key of each instance you pass, with sizes       | live, as the app writes (`addOnValueChangedListener`) |
| SecureStore  | only the keys you declare in `secureStore.keys`       | after each write from Lens, or with **Refresh**       |

SecureStore cannot list its keys, so Lens shows the declared ones; a declared
key the app has not set reads as "not set". A key you delete from Lens leaves
the list until you add it again or the app reloads. Lens lists at most 10,000
keys per store and says so when a store has more.

## Values

Values are read when you open a key, never in bulk. The views:

| View   | Shows                                                    |
| ------ | -------------------------------------------------------- |
| Text   | the value as stored                                      |
| JSON   | a tree, when a string value holds a JSON object or array |
| Hex    | offset, bytes in hex and printable ASCII                 |
| Base64 | the bytes as base64                                      |

MMKV does not record a value's type, so the SDK infers it from what the
getters return: string, number, boolean or bytes (bytes show in Hex first).
A string holding control characters is shown as bytes. AsyncStorage and SecureStore store strings. Values above 1 MiB are
not sent; the panel shows their size.

## Editing

- **Edit** keeps the value's type. When a string value held JSON, Lens refuses
  to save text that is not valid JSON.
- **Add key** writes a new key with a type (numbers, booleans and bytes need
  MMKV).
- **Delete** and **Clear store** ask for a second click to confirm.
- `exclude` keeps keys out of Lens entirely: they are not listed and every
  action on them is refused by the app.

## Export and import

**Export** saves every value of the selected store as a JSON file
(`lens-storage-export`, version 1). **Import** reads such a file and writes
each entry into the selected store; it reports what it wrote and what it
skipped (values that were too large to export, keys the app refuses). Use it
to snapshot a store before a test and restore it afterwards, or to move a
state between simulators.

## Troubleshooting

- **The panel says no app exposes storage**: pass `storage` to `useLens`, and
  check the Queries panel to confirm the SDK is connected at all.
- **MMKV is missing in Expo Go**: Expo Go does not ship MMKV; use a
  development build.
- **A key you expect is missing**: it may match `exclude`, the store may have
  more than 10,000 keys, or the keys are so long that the snapshot was trimmed
  to fit 8 MiB (the store is marked truncated and the SDK logs a warning).
