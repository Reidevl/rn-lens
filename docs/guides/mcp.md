# MCP

Lens serves the [Model Context Protocol](https://modelcontextprotocol.io), so
Claude and other AI clients can read and manage the app Lens is attached to:
its console, network requests, TanStack queries and mutations, storage and
plugin panels, and screenshots.

## Quick path

1. Start Lens (desktop app or `rnlens`). It prints
   `MCP for AI clients at http://127.0.0.1:7431/mcp`.
2. Click **MCP** in Lens's status bar: it copies the command that connects
   Claude Code. Run it:

   ```sh
   claude mcp add --transport http lens http://127.0.0.1:7431/mcp
   ```

3. Ask Claude about your app: "Why does the profile screen show old data?",
   "Clear the session key in AsyncStorage and reload the app", "What did the
   last failed request return?".

Any client that speaks MCP over streamable HTTP connects to the same URL.

## Tools

| Tool                 | Does                                                                                                                                       | Kind        |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ----------- |
| `status`             | Lens version, the attached app and the devices running `@rnlens/sdk`                                                                       | read        |
| `list_targets`       | Apps Metro lists                                                                                                                           | read        |
| `console_logs`       | Console entries, filtered by `level` (verbose, info, warning, error, system; `debug`, `log` and `warn` work too) and `search`, newest last | read        |
| `network_requests`   | Requests, newest first, filtered by URL                                                                                                    | read        |
| `network_request`    | One request with headers, timing and bodies (each cut at 64 KiB)                                                                           | read        |
| `list_queries`       | Queries with their status, without data                                                                                                    | read        |
| `get_query`          | One query with its data                                                                                                                    | read        |
| `list_mutations`     | Mutations                                                                                                                                  | read        |
| `list_stores`        | AsyncStorage, MMKV and SecureStore stores with their keys                                                                                  | read        |
| `get_storage_value`  | One stored value                                                                                                                           | read        |
| `list_plugins`       | The app's [plugin panels](plugins.md) and their blocks                                                                                     | read        |
| `take_screenshot`    | What the app shows, as an image                                                                                                            | read        |
| `attach_target`      | Attaches Lens to an app                                                                                                                    | change      |
| `reload_app`         | Reloads the app's JavaScript (in-memory state is lost)                                                                                     | destructive |
| `set_online`         | Switches TanStack Query's `onlineManager`                                                                                                  | change      |
| `evaluate_js`        | Runs JavaScript in the app                                                                                                                 | destructive |
| `query_action`       | Refetch, invalidate, reset, remove, set data, trigger error or loading states                                                              | destructive |
| `set_storage_value`  | Writes a key                                                                                                                               | destructive |
| `remove_storage_key` | Deletes a key                                                                                                                              | destructive |
| `clear_storage`      | Deletes every key of a store                                                                                                               | destructive |
| `plugin_action`      | Clicks a button of a plugin panel                                                                                                          | destructive |

Read tools carry MCP's read-only hint and the others the destructive hint
when they can change or lose data; clients use these hints in their
permission prompts (Claude Code asks before a tool runs unless you allowed
it). Tools that take a `device` work without it when one device is
connected; with several, pass the key `status` lists (or the app id when it
is unique). Results above 256 KiB are cut: narrow them with `search` or
`limit`.

## Options

| Flag            | Default | Meaning                                                                |
| --------------- | ------- | ---------------------------------------------------------------------- |
| `--mcp-port N`  | `7431`  | Port of the MCP endpoint; `0` turns MCP off                            |
| `--mcp-token T` | none    | Clients must send `Authorization: Bearer T`; prefer `RNLENS_MCP_TOKEN` |

Set the token in the environment rather than on the command line: other
users of the machine can read a process's arguments (`ps`), but not its
environment. Lens warns when the token comes from `--mcp-token`.

```sh
RNLENS_MCP_TOKEN="$(openssl rand -hex 16)" rnlens
```

With a token, add the header to the client:

```sh
claude mcp add --transport http lens http://127.0.0.1:7431/mcp --header 'Authorization: Bearer T'
```

The **MCP** button copies that form when Lens runs with a token. When the port
is taken (another Lens is running), Lens starts without MCP and says so in
its log.

## Security

The endpoint listens on `127.0.0.1` only. It refuses requests whose `Host`
is not localhost (DNS rebinding) and cross-origin requests from web pages,
so a site open in your browser cannot drive it. Anything on this machine can
still reach it: set `RNLENS_MCP_TOKEN` on a shared machine, or use
`--mcp-port 0` to turn it off. The tools can change the app and run JavaScript in it; keep
your AI client's confirmation prompts on for destructive tools.
