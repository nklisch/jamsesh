---
id: epic-portal-api-mcp-endpoint-scaffold-and-tools
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-api-mcp-endpoint
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# MCP Endpoint — Scaffold + 4 Tools

## Scope

Build the streamable-HTTP MCP endpoint with Bearer auth via getServer callback + 4 tools (post_comment, resolve_comment, fork, query_session_state).

## Units delivered

- `internal/portal/mcpendpoint/handler.go` — Endpoint struct + Handler + getServer + tool registration
- `internal/portal/mcpendpoint/tools.go` — 4 tool handler funcs
- `internal/portal/mcpendpoint/handler_test.go`
- `cmd/portal/main.go` (edit) — construct Endpoint, mount via `router.Deps.MountMCP`
- go.mod: add `github.com/modelcontextprotocol/go-sdk`

## Acceptance Criteria

- [ ] `POST /mcp/...` accepts streamable-http per SDK
- [ ] Bad/missing Bearer → SDK returns appropriate auth error
- [ ] post_comment: creates a comment via Comments.Service.Create; emits comment.added event
- [ ] resolve_comment: marks resolved via Comments.Service.Resolve
- [ ] fork: creates ref under jam/<sessionID>/<accountID>/<branch>; upserts ref_modes; emits ref.forked
- [ ] query_session_state: returns {session, unresolved_comments_for_me, open_conflicts_for_me, recent_events}
- [ ] Non-member session: tool returns permission error
- [ ] All tests green

## Notes

- The `mcp-go-sdk` skill carries verified patterns. Use them. The getServer + AddTool pattern is locked.
- Session-membership lookup: walk `ListSessionMembershipsForAccount(account.ID)`, find matching sessionID, extract orgID.
- Use the openapi-generated payload types for event payloads.
