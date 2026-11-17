# Security Policy

## Reporting a vulnerability

Please do not open a public issue for security problems. Use GitHub's private
vulnerability reporting on this repository ("Security" tab, "Report a
vulnerability"). You will get an acknowledgement within 72 hours and a fix or
mitigation plan within 14 days for confirmed issues.

## Threat model

Lens is a developer tool that runs on a developer's machine against apps in
development. It opens local network listeners, so it is worth being explicit
about what it trusts.

| Surface               | What it is                                                                                    | Boundary                                                                                                                                                    |
| --------------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Metro inspector proxy | Lens connects as a Chrome DevTools Protocol client to `ws://<metro-host>/inspector/debug`     | Same trust as React Native DevTools: anyone who can reach Metro can already evaluate code in the app                                                        |
| SDK WebSocket server  | `@rnlens/sdk` connects from the app to Lens (default port 7420) to stream queries and storage | Listens on all interfaces so physical devices can connect; validates the handshake, rejects unknown protocol majors, and supports an optional pairing token |
| Console eval          | Runs JavaScript in the app on request                                                         | Only ever triggered by the user in the Lens window                                                                                                          |
| Storage editing       | Writes values into the app's storage through the SDK                                          | The SDK is a no-op in production bundles (`__DEV__` guard)                                                                                                  |

Known limits, by design:

- Lens does not encrypt the local WebSocket. Use it on networks you trust, the
  same way you would use Metro.
- Lens never sends data anywhere except the connected app and your screen. There
  is no telemetry.

## Supported versions

Only the latest minor release receives security fixes while the project is pre-1.0.
