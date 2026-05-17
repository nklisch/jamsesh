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

## Design decisions

- **WebSocket library**: `nhooyr.io/websocket`. Modern, context-aware API
  designed around Go's `context.Context` cancellation; simpler surface
  than `gorilla/websocket`; smaller dependency footprint. Mature; the
  gorilla project recommends nhooyr for new code.

- **MCP server library**: `github.com/modelcontextprotocol/go-sdk` v1.x
  (official Anthropic/Google SDK, v1.6.0+ as of May 2026). Drop-in chi
  mount:
  ```go
  handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
      if !validBearer(r.Header.Get("Authorization")) { return nil }
      s := mcp.NewServer(&mcp.Implementation{Name: "jamsesh", Version: "0.1"}, nil)
      mcp.AddTool(s, &mcp.Tool{Name: "post_comment", ...}, postComment)
      // ... 3 more tools
      return s
  }, nil)
  mux.Mount("/mcp", handler)
  ```
  Reasons over alternatives: (1) `getServer(*http.Request)` callback is the
  cleanest fit for "inspect Bearer token, then dispatch" — no middleware
  gymnastics; (2) v1.x stable API (mark3labs/mcp-go is still 0.x); (3)
  typed-struct tool registration via generics gives less boilerplate for
  our 4 tools; (4) first-party so spec changes land fast. Tracks MCP spec
  2025-06-18 (the November 2025 release line). Fallback only on a v1.6 bug:
  `mark3labs/mcp-go` is a safe second choice.

- **WebSocket event-envelope schema versioning**: bake `version: 1` into
  every envelope from day one. Envelope shape: `{seq, version, type,
  payload, timestamp, session_id}`. Clients can branch on version when
  we evolve. Cheap forward-compat for a long-lived event stream.

- **Pagination model**: cursor-based for all list endpoints. Response
  shape: `{items: [...], next_cursor: "<opaque>"}`. Stable under inserts
  (the event log gets entries added constantly; offset pagination
  would drift). Cursor is opaque server-side state (typically a base64
  of `last_seen_id + filter_hash`). Applies to digest endpoint, comments
  list, sessions list, refs list.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


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
