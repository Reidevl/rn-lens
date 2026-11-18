# Performance

The Performance panel records what the attached app does: a screenshot, a
trace for Perfetto UI or Chrome DevTools, a JavaScript CPU profile and a
Hermes heap snapshot. The status bar shows the last Fast Refresh. No code in
the app is needed.

## Quick path

1. Run your app in development (a development build or Expo Go) and start
   Metro.
2. Attach to it from the Targets panel.
3. Open **Performance** and pick a card.

## Screenshot

**Capture** takes a picture of what the app shows. Choose PNG (default) or
JPEG. **Save…** writes the image where you choose; **Copy image** puts a PNG
on the clipboard.

## Trace

1. Pick the categories to record. The defaults match what React Native
   DevTools records:

   | Category             | Records                                                 |
   | -------------------- | ------------------------------------------------------- |
   | JavaScript sampling  | Where the JavaScript thread spends its time             |
   | JavaScript execution | Each JavaScript task                                    |
   | Timeline             | Rendering and scheduling events                         |
   | Timeline (detailed)  | The detailed variant of the timeline                    |
   | Frames               | Frame boundaries                                        |
   | User timing          | Your `performance.mark` and `performance.measure` calls |
   | Screenshots          | Off by default: frames captured while recording         |

2. **Start recording**, use the app, then **Stop**. Lens waits until the app
   has sent every chunk, then shows the duration, the number of events and
   the file size.
3. **Save trace…** writes a Chrome JSON trace (`{"traceEvents": [...]}`).
   Open it at [ui.perfetto.dev](https://ui.perfetto.dev) (**Open trace
   file**) or in Chrome DevTools (**Performance → Load profile**).

React Native records one trace at a time. The app refuses to trace when more
than one React Native host is registered, or while a debugger has the
Debugger domain enabled (React Native DevTools does, while it is open);
Lens shows the app's message. "The app dropped some events" means the app's
trace buffer filled up: record a shorter session.

## CPU profile

**Start profiling**, use the app, then **Stop**. **Save profile…** writes a
`.cpuprofile` file that Chrome DevTools (**Performance → Load profile**) and
[speedscope](https://www.speedscope.app) open.

## Heap snapshot

**Take snapshot…** asks where to save, then streams the Hermes heap there as
a `.heapsnapshot` file. Load it in Chrome DevTools: **Memory → Load
profile**. A snapshot of a small app is a few megabytes; the app pauses while
it is taken.

## Fast Refresh

After attaching, Lens reads the URL the app loaded its bundle from and
registers it with Metro's hot reloading socket (`/hot`), as the app does. The
status bar then shows:

| Status bar                                           | Meaning                                               |
| ---------------------------------------------------- | ----------------------------------------------------- |
| Fast Refresh ready                                   | Registered; no change since attaching                 |
| Fast Refresh updating…                               | Metro is rebuilding after a file change               |
| Fast Refresh: 3 modules just now                     | The last update and how many modules it changed       |
| Fast Refresh failed                                  | The rebuild failed; hover for Metro's error           |
| Fast Refresh unavailable                             | The app does not load its bundle from Metro (release) |
| Fast Refresh unavailable, "Reload the app…" on hover | Metro restarted since the app loaded; reload the app  |

Lens reconnects when Metro restarts and stops when you detach.

A recording ends when the debugger connection does: detaching, a reload
that reconnects, or a dropped connection all stop it, and its data is lost.
Stop and save before reloading the app.

## Troubleshooting

| Symptom                                                                             | Fix                                                                                                                |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| "The Tracing domain is unavailable when multiple React Native hosts are registered" | Close the other React Native instances in the app, or trace another app                                            |
| Trace start fails while React Native DevTools is open                               | Close React Native DevTools, then record                                                                           |
| Fast Refresh never updates after you save a file                                    | Metro is not watching files: it was started with `CI=1` ("reloads are disabled" in its log); start it without `CI` |
