---
title: Why is this screen slow?
description: Find what the JavaScript thread does while a screen feels slow.
---

1. Attach Lens to the app and open **Performance**.
2. In **Trace**, keep the default categories and click **Start recording**.
3. Do the slow thing in the app: open the screen, scroll the list.
4. Click **Stop**, then **Save trace…**.
5. Open [ui.perfetto.dev](https://ui.perfetto.dev) and drop the file in.

In Perfetto, the JavaScript thread's long slices are the work that blocked
the screen. Mark your own spans with `performance.mark()` and
`performance.measure()`; they show up as **User timing**.

For a focused view of JavaScript only, record a **CPU profile** instead and
open the `.cpuprofile` in Chrome DevTools or [speedscope](https://www.speedscope.app).

Memory growing over time? Take a **heap snapshot** before and after the
action and compare them in Chrome DevTools (**Memory → Load profile**).

Details: [Performance](/guides/performance/).
