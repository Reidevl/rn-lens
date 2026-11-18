---
title: Reproduce a bug from someone's stored state
description: Move AsyncStorage, MMKV and SecureStore contents between devices.
---

1. Add `useLens({ storage: { asyncStorage, mmkv, secureStore } })`
   ([Storage](/guides/storage/)).
2. On the device that shows the bug, open **Storage**, pick the store and
   click **Export**. Send the file along with the bug report.
3. On your device, open the same store and click **Import**: the keys and
   values are written into the app.
4. Reload the app and step through the bug.

You can also edit a value in place — JSON values are checked before they
are saved — or delete a key to test a first-launch path.
