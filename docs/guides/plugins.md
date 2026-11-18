# Plugins

The Plugins panel shows panels your app adds to Lens: feature flags, a Redux
or Zustand store, a job queue, a cache, anything the app knows. You describe a
panel with building blocks and Lens draws it with its own components. No code
of yours runs inside Lens, and the panel works the same in the desktop app and
in [browser mode](browser-mode.md).

## Quick path

1. Install `@rnlens/sdk` and call `useLens` (see the [SDK README](../../packages/sdk/README.md)).
2. Create a panel and pass it to `useLens`:

   ```tsx
   import { createLensPanel, useLens } from "@rnlens/sdk";

   const flags = { newCheckout: false, darkMode: false };

   const flagsPanel = createLensPanel({
     id: "feature-flags",
     name: "Feature flags",
     render: () => [
       {
         type: "table",
         id: "flags",
         columns: [
           { key: "name", label: "Flag" },
           { key: "on", label: "On" },
         ],
         rows: Object.entries(flags).map(([name, on]) => ({ id: name, cells: { name, on } })),
         rowActions: [{ id: "toggle", label: "Toggle" }],
       },
     ],
     onAction: ({ rowId }) => {
       const flag = rowId as keyof typeof flags;
       flags[flag] = !flags[flag];
       return `${flag} is ${flags[flag] ? "on" : "off"}`;
     },
   });

   useLens({ panels: [flagsPanel] });
   ```

3. Open **Plugins** in Lens. Each panel is a tab; with several apps or
   devices, pick one in the device menu.

The example app (`examples/expo-app/panels.ts`) has a complete panel with a
table, buttons and a log.

## Building blocks

| Block      | Shows                                  | Fields                                                              |
| ---------- | -------------------------------------- | ------------------------------------------------------------------- |
| `text`     | A paragraph                            | `text`, `tone?`: `default`, `muted`, `warning`, `error`             |
| `keyValue` | Labelled values                        | `items: [{ key, label?, value }]`                                   |
| `table`    | Rows and columns, with buttons per row | `columns: [{ key, label }]`, `rows: [{ id, cells }]`, `rowActions?` |
| `json`     | A collapsible JSON tree                | `value`                                                             |
| `log`      | Entries, newest last                   | `entries: [{ text, level?, time? }]`                                |
| `actions`  | Buttons                                | `actions: [{ id, label, destructive?, confirm? }]`                  |

Every block has an `id`, unique in the panel, and an optional `title`. Values
can be any JSON value: objects and arrays show as trees, other values inline.

## Actions

A button calls `onAction({ blockId, actionId, rowId? })` in the app; `rowId`
is set for a table row button.

- Return a short message: Lens shows it under the tabs.
- Throw an error: Lens shows its message as an error.
- `confirm: "Reset every flag?"` makes Lens ask before sending;
  `destructive: true` draws the button in red.

After each action the panel renders again. When its data changes for another
reason, call `flagsPanel.refresh()`; refreshes within 16 ms send one view.

## Limits

| Limit              | Value | When exceeded                                |
| ------------------ | ----- | -------------------------------------------- |
| View size          | 1 MiB | The panel shows an error instead of its view |
| Blocks per panel   | 100   | Same                                         |
| An action's answer | 15 s  | Lens shows a timeout error                   |

The protocol schema also caps a table at 5,000 rows and a log at 1,000
entries; Lens does not check those, but a view that large rarely fits in
1 MiB anyway.

`render` that throws shows "The panel failed to render" with its message.
Values JSON cannot carry are sent like query data: BigInt and Date as
strings, circular references as `"[Circular]"`.

## Troubleshooting

| Symptom                                                     | Fix                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| "No app has added a panel to Lens yet."                     | Pass `panels` to `useLens`, and check the app is connected (Queries or Storage show it) |
| A tab shows "This block (…) needs a newer version of Lens." | The SDK is newer than Lens: update Lens                                                 |
| `invalid panel id` thrown at startup                        | Use lowercase letters, digits, `.`, `_` or `-` for `id`                                 |
