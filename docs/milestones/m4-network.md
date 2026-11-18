# M4 Network

**Objective:** a Chrome-like Network panel for the attached app over the
Chrome DevTools Protocol (React Native 0.83+): request table with a
waterfall, detail tabs, lazy bodies, copy as cURL and HAR export.

## Scope

| Area     | Delivered                                                                                                                                                                                     |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Protocol | Hand-written Network vocabulary (requestWillBeSent, responseReceived, dataReceived, loadingFinished, loadingFailed, getResponseBody, getRequestPostData) with lenient headers                 |
| Core     | `internal/network`: versioned store of 2,000 requests correlated by `requestId`, redirects as hops, epochs per attach; service with availability, lazy bodies in a 32 MiB LRU; HAR 1.2 export |
| Desktop  | `NetworkService` (availability, stats, since, request, bodies, clear, save HAR through the native dialog) and the `network:changed` coalesced event                                           |
| Frontend | Network panel: notify-then-pull by version, virtualized table with waterfall, type chips, URL filter, preserve log, detail tabs, copy as cURL, export HAR, unsupported and empty states       |
| Docs     | [Network guide](../guides/network.md)                                                                                                                                                         |

## Acceptance checks

- [x] Attaching enables `Network`; an app older than 0.83 shows "Network inspection needs RN 0.83+"
- [x] A request to `https://httpbin.org/json` on RN 0.83+ (development build) shows a row with 200 and its body; e2e `TestE2E_NetworkCapturesARequest`. Expo Go reports no traffic, and Expo SDK 58+ `fetch` needs `EXPO_PUBLIC_USE_RN_FETCH=1` (see the guide)
- [x] Rows update in place as the response, size and end arrive; only changed rows are pulled
- [x] HAR export opens in Chrome DevTools and other HAR viewers (golden test pins the format)
- [x] Independent concurrency review of the store and service (two blockers fixed with regression tests)

## Out of scope

WebSocket frames (canary in React Native 0.88), request blocking, throttling
and mocking (not exposed by React Native), cookies (not reported by the app).
