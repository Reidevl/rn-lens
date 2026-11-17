# How Lens connects to your app

Lens attaches to a React Native app the same way React Native DevTools does,
through Metro. Nothing needs to be installed in the app for the Targets,
Console and Network panels.

## Quick path

1. Start your app with Metro running: `npx expo start` or `npx react-native start`.
2. Open Lens. The app appears under **Targets** within a second or two.
3. Click **Attach**. The status bar turns green and shows the app, platform and React Native version.

## What happens underneath

| Step      | Mechanism                                                                                                                                                                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Discovery | Lens polls `GET /json` on `localhost:8081`, `8082` and `19000` every second. Each entry is a debuggable page with its WebSocket URL and capability flags.                                                                                                          |
| Attach    | Lens opens the page's `ws://…/inspector/debug?device=…&page=…` socket. Metro only accepts the upgrade when the `Origin` header equals its own base URL, so Lens sends `http://127.0.0.1:<port>` for loopback hosts.                                                |
| Handshake | Before any other command Lens sends `ReactNativeApplication.enable`. React Native 0.76+ treats such a client as a first-class DevTools frontend and replies with `metadataUpdated`: app name, bundle id, device, platform, React Native version and feature flags. |
| Reload    | **Reload app** sends `Page.reload`. Modern ("reloadable") targets keep the same page across reloads, so the session stays attached.                                                                                                                                |
| Hand-off  | The ↗ button posts to Metro's `/open-debugger?target=…`, which launches React Native DevTools for the same page.                                                                                                                                                   |

## Reconnection rules

| Situation                              | What Lens does                                                                                                                                                                         |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Metro restarts or the app process dies | The status bar shows **Reconnecting** with the attempt count. Lens retries with backoff (0.5 s doubling up to 5 s) until the target is back or you detach.                             |
| Another debugger takes the page        | Metro closes the socket with `[NEW_DEBUGGER_OPENED]`. Lens shows **Disconnected by another debugger** and waits for you to **Attach again**, so two clients never fight over one slot. |
| You detach                             | The session closes and nothing reconnects.                                                                                                                                             |

## Exclusive attach

A target advertises `supportsMultipleDebuggers`. Metro only grants it when it
runs the standalone DevTools shell, the default since React Native 0.83. On
older versions the badge **exclusive attach** appears: attaching Lens evicts
React Native DevTools and vice versa. Use the hand-off button when you need
the official debugger, then attach Lens again.

## Headless use

```bash
rnlens targets                       # JSON list of targets; exit 2 when none
rnlens record -out capture.jsonl     # attach and log every CDP frame for 30 s
```

Recordings feed the fake proxy used by the test suite.
