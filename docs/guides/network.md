# Network

The Network panel lists the HTTP requests your app makes, as React Native
reports them through the Chrome DevTools Protocol. No code in the app is
needed.

## Quick path

1. Run your app on React Native 0.83 or newer and start Metro.
2. Attach to it from the Targets panel.
3. Open **Network** and make a request in the app. Expo apps: read the Expo
   notes below first.

## Requirements

Native network inspection shipped in React Native 0.83. On older versions the
panel shows "Network inspection needs RN 0.83+". When the app reports any
other problem while enabling the domain (for example "The Network domain is
unavailable when multiple React Native hosts are registered"), the panel
shows that message instead.

### Expo

- **Use a development build** (`npx expo run:ios` or `npx expo run:android`).
  Expo Go accepts the connection but reports no requests: React Native only
  reports traffic when its networking code is compiled with the debugger
  enabled, and the prebuilt Expo Go client is not.
- **`fetch` in Expo SDK 58+** is `expo/fetch`, which does not report to any
  debugger. `XMLHttpRequest` and libraries built on it (axios) are reported.
  To see `fetch` calls too, start Metro with React Native's fetch:

  ```sh
  EXPO_PUBLIC_USE_RN_FETCH=1 npx expo start --dev-client
  ```

## The table

| Column    | Meaning                                                            |
| --------- | ------------------------------------------------------------------ |
| Name      | Last path segment and query; hover for the full URL                |
| Method    | HTTP method                                                        |
| Status    | Status code, `Pending`, `(failed)` or `(canceled)`                 |
| Type      | Resource type the app reported (`Fetch`, `XHR`, `Image`, …)        |
| Size      | Bytes on the wire when known, otherwise the decoded size           |
| Time      | From request start to the last byte                                |
| Waterfall | When the request ran, on one time axis shared by every visible row |

The newest request is on top. Click a column header to sort by it; click it
again to reverse the order (the Waterfall header sorts by start time). While a
request's details are open the table keeps only Name, Status and Time.

Filter by type with the chips (Fetch/XHR, Doc, JS, Img, Media, Font, WS,
Other) and by URL with the search box. Requests from earlier sessions (before
a reload or re-attach) are hidden unless **Preserve log** is on. **Clear**
empties the list. Lens keeps the latest 2,000 requests; the footer says how
many older ones were dropped.

## Request details

Selecting a row opens its details:

| Tab       | Shows                                                                                     |
| --------- | ----------------------------------------------------------------------------------------- |
| Headers   | URL, method, status, remote address, protocol, response and request headers               |
| Payload   | Query string parameters and the request body                                              |
| Preview   | JSON as a tree, images rendered, other text as-is                                         |
| Response  | The raw response body, with a Copy button                                                 |
| Timing    | Queueing, DNS, connection, SSL, request sent, waiting for the server and content download |
| Initiator | What started the request, with its stack when the app reports one                         |

Bodies are fetched from the app only when you open a tab that needs them, and
cached (up to 32 MiB). Bodies larger than 5 MiB are not shown. Bodies of
requests from an earlier session cannot be fetched: React Native keeps them
per debugger connection.

React Native on iOS sends every body base64-encoded; Lens decodes text bodies
(JSON, XML, `text/*`, form data) so Preview and Response show them as text,
and keeps images and other binary bodies as they are.

**Copy response** puts the response body on the clipboard from any tab (text
as received; base64 for binary bodies).

**Copy as cURL** puts a POSIX shell command on the clipboard with the method,
headers and body of the request.

## Export HAR

**Export HAR** saves every captured request as a HAR 1.2 file, with the
bodies Lens can still fetch. Open it in Chrome DevTools (drop it on the
Network tab), Charles, Proxyman or any HAR viewer, or attach it to a bug
report.

## Limits

- WebSocket frames, request blocking, throttling and mocking are not
  available: React Native does not expose them yet.
- Cookies are not listed separately; they appear in the headers when the app
  reports them.
