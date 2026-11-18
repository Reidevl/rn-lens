---
title: Why does the screen show old data?
description: Inspect TanStack Query caches and force the states you need to test.
---

1. Add `useLens({ queryClient })` to the app ([Queries](/guides/queries/)).
2. Open **Queries**: each query shows its status — fresh, stale, fetching,
   paused or inactive — its data and when it last updated.
3. Act on the query from Lens:
   - **Invalidate** or **Refetch** to see whether new data arrives.
   - **Trigger loading** and **Trigger error** to check the screen's loading
     and error states without touching the server.
   - **Edit data** to try an edge case the API rarely returns.
4. Toggle **offline** in the status bar to test how the app behaves when
   `onlineManager` says the device is offline.

Mutations get the same view in **Mutations**.
