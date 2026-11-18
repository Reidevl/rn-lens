# Network

The Network panel lists the HTTP requests your app makes, as React Native
reports them through the Chrome DevTools Protocol. No code in the app is
needed.

## Quick path

1. Run your app on React Native 0.83 or newer and start Metro.
2. Attach to it from the Targets panel.
3. Open **Network** and make a request, for example from the console:
   `fetch('https://httpbin.org/json')`.

## Requirements

Native network inspection shipped in React Native 0.83. On older versions the
panel shows "Network inspection needs RN 0.83+". When the app reports any
other problem while enabling the domain (for example more than one React
Native host registered in the process), the panel shows that message instead.

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
