# M5 Storage

**Objective:** a Storage panel for the app's AsyncStorage, MMKV and
SecureStore through `@rnlens/sdk`: browse, search, view as text, JSON, hex or
base64, edit, add, delete, export and import.

## Scope

| Area     | Delivered                                                                                                                                                                 |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Protocol | Additive minor of SDK protocol v1: `storage.snapshot`, `storage.event`, `storage.action`, `storage.result`; values as `{type, value}`; fixtures and two-sided conformance |
| SDK      | `useLens({ storage })` with AsyncStorage, MMKV (v3 and v4, live) and SecureStore (declared keys); `exclude`; 1 MiB values and 10,000 keys per store                       |
| Core     | `internal/storage`: per-device mirror of stores and keys, validated actions with their own result message, export and import                                              |
| Desktop  | `StorageService` (devices, stores, get, set, remove, clear, refresh, export and import through native dialogs) and the `storage:changed` event                            |
| Frontend | Storage panel: device picker, store tabs, searchable key list, value views, editor, add, delete, clear, export and import                                                 |
| Docs     | [Storage guide](../guides/storage.md), SDK README, example app                                                                                                            |

## Acceptance checks

- [x] Edit an AsyncStorage key in Lens → `getItem` in the app returns the new value (`TestE2E_StorageEditReachesTheApp`, green on an RN 0.88.1 development build)
- [x] MMKV changes made by the app appear without a refresh
- [x] SecureStore lists only the declared keys
- [x] Export then import restores a store
- [x] Independent review of the protocol contract and the action round-trip

## Out of scope

Encrypted MMKV key management, SQLite and other databases (plugin
candidates), watching AsyncStorage for changes the app makes (it has no
listener; use Refresh).
