---
id: epic-cc-plugin-session-commands-fork-and-mode
kind: story
stage: implementing
tags: [plugin]
parent: epic-cc-plugin-session-commands
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Session Commands — Fork + Mode

## Scope

`jamsesh fork` (calls MCP `fork` tool) and `jamsesh mode` (v1: updates local state with TODO for server-side).

## Units delivered

- `cmd/jamsesh/sessioncmd/fork.go` + test
- `cmd/jamsesh/sessioncmd/mode.go` + test
- `cmd/jamsesh/mcpclient/client.go` — minimal JSON-RPC client for calling /mcp tools (for fork)
- `cmd/jamsesh/main.go` (edit) — register subcommands

## Acceptance Criteria

- [ ] `jamsesh fork <commit-sha>`: calls MCP `fork` tool via /mcp HTTP endpoint with Bearer auth; on success runs local `git fetch` to pull the new ref
- [ ] `jamsesh fork --as <branch> --mode isolated`: parameters propagate correctly to MCP
- [ ] `jamsesh mode sync|isolated`: writes new mode to local state file `${PluginDataDir}/sessions/<sid>/mode`; prints "(server-side mode change pending v1 follow-up)"
- [ ] Tests mock portal MCP endpoint with httptest

## Notes

- The mcpclient is tiny: build a JSON-RPC 2.0 request body `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fork","arguments":{...}}}` and POST to `<portal_url>/mcp` with Authorization Bearer. Parse the response.
- v1 limitation on mode: server-side mode change is a follow-up. The CLI updates local cache only.
