# M8 MCP

**Objective:** let Claude and other AI clients read and manage the data of the
app Lens is attached to, over the Model Context Protocol.

## Scope

| Area     | Delivered                                                                                                                                 |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| Server   | `internal/adapters/mcpserver`: 21 tools over streamable HTTP on `127.0.0.1:7431/mcp` (MCP Go SDK v1.8.0), read-only and destructive hints |
| Security | Localhost only, DNS-rebinding and cross-origin protection, optional bearer token                                                          |
| CLI      | `--mcp-port` (0 turns it off), `--mcp-token`, in the desktop and browser builds                                                           |
| UI       | **MCP** in the status bar copies the `claude mcp add` command                                                                             |
| Docs     | [MCP guide](../guides/mcp.md)                                                                                                             |

## Acceptance checks

- [x] Every tool through an in-memory MCP client against a fake Lens; the HTTP endpoint with token, cross-origin and Host checks
- [x] `TestE2E_MCPManagesTheApp` on an RN 0.88.1 development build: attach, evaluate, invalidate a query, write, read and remove a storage key, click a plugin button, screenshot
- [ ] Independent review of the endpoint's security and the tool contracts

## Out of scope

MCP resources and prompts, remote (non-localhost) access, and OAuth.
