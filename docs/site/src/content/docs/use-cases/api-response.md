---
title: Is the API returning what I think?
description: Check a request, its body and its timing, then replay it outside the app.
---

1. Attach Lens and open **Network** (React Native 0.83 or newer).
2. Trigger the request in the app. The newest request is on top.
3. Select it: **Headers**, **Payload**, **Preview** and **Response** show what
   went out and what came back; **Timing** shows where the time went.
4. **Copy as cURL** replays the same request in a terminal, so you can tell an
   app bug from a server bug.
5. **Export HAR** saves the whole session for a teammate or a bug report.

Expo apps: use a development build, and start Metro with
`EXPO_PUBLIC_USE_RN_FETCH=1` to see `fetch` calls.

Details: [Network](/guides/network/).
