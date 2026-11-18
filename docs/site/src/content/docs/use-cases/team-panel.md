---
title: Give your team a panel for your app
description: Show feature flags, a store or a queue in Lens, with buttons that act on the app.
---

Every app has state worth seeing: feature flags, a Redux or Zustand store, an
upload queue, the current user. Instead of logging it, describe a panel:

```ts
import { createLensPanel, useLens } from "@rnlens/sdk";

const queuePanel = createLensPanel({
  id: "uploads",
  name: "Uploads",
  render: () => [
    {
      type: "table",
      id: "queue",
      columns: [
        { key: "file", label: "File" },
        { key: "state", label: "State" },
      ],
      rows: uploads.map((u) => ({ id: u.id, cells: { file: u.name, state: u.state } })),
      rowActions: [{ id: "retry", label: "Retry" }],
    },
  ],
  onAction: ({ rowId }) => retryUpload(rowId!),
});

useLens({ panels: [queuePanel] });
```

Call `queuePanel.refresh()` when the queue changes. Everyone who attaches
Lens to the app sees the panel, in the desktop app and in the browser.

Details: [Plugins](/guides/plugins/).
