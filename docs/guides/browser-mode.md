# Browser mode

Lens can run in a browser tab instead of a desktop window. It is the same
Lens, built as a local web server: every panel works, and files you save are
downloaded by the browser.

## Quick path

```sh
task build:server          # builds bin/rnlens-server
./bin/rnlens-server        # opens http://localhost:7430
```

The terminal prints `Lens is running at http://localhost:7430`. Quit with
Ctrl+C.

## Options

| Flag          | Default                       | Meaning                                            |
| ------------- | ----------------------------- | -------------------------------------------------- |
| `--port N`    | `7430`                        | Port of the Lens UI                                |
| `--host H`    | `localhost`                   | Address to listen on (see Security)                |
| `--no-open`   | off                           | Print the URL without opening a browser            |
| `--metro`     | `localhost:8081, 8082, 19000` | Comma-separated Metro base URLs to poll            |
| `--sdk-port`  | `7420`                        | Port of the `@rnlens/sdk` server (`0` disables it) |
| `--sdk-token` | none                          | Pairing token the SDK must present                 |

`--metro` and the SDK flags work the same in the desktop app.

## What differs from the desktop app

| Action                                                                | Browser mode                              |
| --------------------------------------------------------------------- | ----------------------------------------- |
| Save (HAR, storage export, screenshot, trace, profile, heap snapshot) | The browser downloads the file            |
| Storage import                                                        | The browser's file picker, then an upload |
| Copy buttons                                                          | The browser clipboard                     |

## Security

Lens listens on `localhost` by default, so only this machine reaches it, and
it refuses requests whose `Host` header is not that address. `--host 0.0.0.0`
makes it reachable from your network: anyone who can reach the port can read
your app's logs, requests, queries and storage, and change storage. Lens
prints a warning when you do this.
