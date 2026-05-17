---
id: epic-cc-plugin-session-commands-fork-and-mode
kind: story
stage: done
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

## Implementation notes

All files were already committed by the hooks sibling story (`epic-cc-plugin-hooks-retry-queue-and-simple-hooks`), which scaffolded the full sessioncmd and mcpclient packages as part of wiring main.go. This story verified, tested, and finalized the implementations:

- `cmd/jamsesh/mcpclient/client.go` — JSON-RPC 2.0 client; `CallTool` POSTs to `<PortalURL>/mcp`, unwraps `StructuredContent` from the SDK envelope. Tests: `TestCallTool_success`, `TestCallTool_httpError`, `TestCallTool_rpcError` all pass.
- `cmd/jamsesh/sessioncmd/session.go` — `resolveSession()` maps `CC_SESSION_ID` env var to a jamsesh session ID via `${CLAUDE_PLUGIN_DATA}/sessions/<id>/instance_id`; falls back to first session directory for single-session dev.
- `cmd/jamsesh/sessioncmd/fork.go` — `ForkCommand()` calls MCP `fork` tool with `session_id`, `target_commit_sha`, optional `target_ref`+`mode`; on success runs `git fetch session-remote <ref>` (non-fatal if fetch fails). Tests pass.
- `cmd/jamsesh/sessioncmd/mode.go` — `ModeCommand()` validates `sync|isolated`, writes to `${CLAUDE_PLUGIN_DATA}/sessions/<id>/mode`, prints v1 limitation note. Tests pass.
- `cmd/jamsesh/main.go` — `ForkCommand()` and `ModeCommand()` registered (alongside `JoinCommand()`, `StatusCommand()` added by sibling).

Pre-existing test failures in `join_test.go` (TestJoinAction_happy, TestJoinAction_inviteURL) and `status_test.go` are from the join-and-status sibling story and not related to this story's scope.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Tiny JSON-RPC mcpclient with StructuredContent unwrapping is the right minimal surface. Mode v1 local-only with clear print message about server-side follow-up is honest about scope.
