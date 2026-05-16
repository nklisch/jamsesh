---
id: epic-portal-api
kind: epic
stage: drafting
tags: [portal]
parent: null
depends_on: [epic-portal-foundation, epic-portal-git, epic-auto-merger]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal API (REST + MCP + WebSocket)

## Brief

The portal's external interfaces. Three transports sharing one auth model
(user OAuth bearer token) and one data layer:

**REST API** for session lifecycle (create, list, get, finalize, abandon),
invitations (create, accept, remove members), comments (list, resolve —
posting is via MCP), the digest endpoint that the local binary calls at
turn start, and the finalize plan generation endpoint.

**MCP endpoint** (HTTPS-MCP via `streamable-http` transport) exposing the
four jamsesh tools as thin proxies to REST handlers: `post_comment`,
`resolve_comment`, `fork`, `query_session_state`. Every tool call carries
`session_id` so session-scoped authorization checks fire.

**WebSocket gateway** for real-time push to portal UI clients. Per-session
subscription model. Event types: `commit.arrived`, `merge.succeeded`,
`conflict.detected`, `conflict.resolved`, `comment.added`, `comment.resolved`,
`ref.forked`, `mode.changed`, `turn.ended`, `presence.updated`,
`session.finalizing`, `session.ended`.

Backed by the event log (chronological per-session events with monotonic
sequence numbers) that feeds both the digest (REST poll, cursor-based) and
the WebSocket gateway.

This epic does NOT cover any UI work (`epic-portal-ui` consumes this); it
does NOT cover the local plugin (`epic-cc-plugin` consumes this); it does
NOT cover finalize curation UI (`epic-finalize-flow` handles the
cross-component slice).

## Foundation references

- `docs/ARCHITECTURE.md` — Portal (REST API, MCP endpoint, WebSocket gateway
  subcomponents)
- `docs/PROTOCOL.md` — MCP tools, REST API, WebSocket event types, HTTP
  error contract
- `docs/SECURITY.md` — MCP and REST API authorization

## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Session CRUD endpoints (create, list, get, patch, finalize, abandon)
- Invitations (create, accept, remove members)
- Comments endpoints (list, resolve, anchor model with line ranges)
- Digest endpoint (cursor-based event log query, formatted output for
  `additionalContext`)
- MCP endpoint setup (streamable-http transport, auth, tool registration)
- MCP `post_comment` tool
- MCP `resolve_comment` tool
- MCP `fork` tool (server-side ref manipulation with policy validation)
- MCP `query_session_state` tool
- WebSocket gateway (subscription model, fanout)
- Event log model + emission helpers
- Finalize plan generation (computes the cherry-pick script)

<!-- Design pass on each child feature will fill in specifics. -->
