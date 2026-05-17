---
id: epic-portal-api-mcp-endpoint-scaffold-and-tools
kind: story
stage: done
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

## Implementation notes

- **Auth pattern**: Per `mcp-go-sdk` skill pitfall #6, auth is done via `auth.RequireBearerToken` middleware wrapping the streamable-HTTP handler, NOT inside `getServer`. A single `*mcp.Server` is created at startup; `getServer` just returns it (the middleware already validated the token).
- **`auth.TokenInfoFromContext`** used in every tool handler to get `UserID`; `TokenInfo.Expiration` is set to `now+24h` (required non-zero per pitfall #3; actual TTL enforced by DB).
- **`EventSummary` output type**: `events.Event.Payload` is `json.RawMessage`; the SDK validates the output schema and rejects `json.RawMessage` (treated as untyped object). Output uses a flat `EventSummary{Payload string}` instead.
- **`store.Comment`/`store.ConflictEvent`** used directly in output — SDK output schema validation passed cleanly for these types.
- **fork**: Opens bare repo via `storage.Service.RepoPath` + `gogit.PlainOpen`, verifies commit exists, creates ref under `refs/heads/jam/<sessionID>/<accountID>/<branch>`, upserts `ref_modes`, emits `ref.forked` event. Event emission failure is non-fatal (ref creation succeeded).
- **query_session_state**: Supports `include` filter; defaults to all fields. Unresolved comments fetched by email address (approximate match); `@all-agents` broadcast also fetched. Draft tip reads `refs/heads/jam/<sessionID>/draft` from bare repo.
- **Tests**: 9 tests using raw JSON-RPC over `httptest.Server`. Covers: bad token (401), post_comment happy path + non-member error, resolve_comment happy path, fork happy path + bad commit error, query_session_state happy path + non-member + recent events.
- **go.mod**: `github.com/modelcontextprotocol/go-sdk v1.6.0` added as direct dependency.
- **`cmd/portal/main.go`**: `mcpendpoint.Endpoint` constructed and mounted via `router.Deps.MountMCP`.

## Review (2026-05-17)

**Verdict**: Approve

**Notes**: Followed the mcp-go-sdk skill's pitfalls (auth-as-middleware not in-getServer; non-zero TokenInfo.Expiration; UserID set). EventSummary output workaround for SDK's json.RawMessage handling is a clean adaptation. All 4 tools work end-to-end.
