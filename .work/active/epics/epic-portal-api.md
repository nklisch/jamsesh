---
id: epic-portal-api
kind: epic
stage: done
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

- **WebSocket library**: `github.com/coder/websocket` (the active fork of
  `nhooyr.io/websocket`; nhooyr handed off stewardship to Coder in 2024).
  Modern, context-aware API designed around Go's `context.Context`
  cancellation; simpler surface than `gorilla/websocket`; smaller dependency
  footprint. Pin v1.8.x.

- **MCP server library**: `github.com/modelcontextprotocol/go-sdk` v1.6.0
  (official Anthropic/Google SDK; pin exact in `go.mod`). Drop-in chi mount
  with the canonical `auth.RequireBearerToken` middleware:
  ```go
  // Build a fresh mcp.Server per request via the streamable-http handler.
  handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
      s := mcp.NewServer(&mcp.Implementation{Name: "jamsesh", Version: "0.1"}, nil)
      mcp.AddTool(s, &mcp.Tool{Name: "post_comment", ...}, postComment)
      // ... 3 more tools
      return s
  }, nil)

  // RequireBearerToken validates the Authorization header, produces a proper
  // 401 with `WWW-Authenticate: Bearer resource_metadata="..."` per RFC 9728,
  // and stashes *auth.TokenInfo into the request context (retrievable in
  // tool handlers via auth.TokenInfoFromContext(ctx)). TokenInfo MUST carry
  // a non-empty UserID (enables SDK session-hijack protection on the
  // Mcp-Session-Id header) and a non-zero Expiration (silent 401 otherwise).
  authed := auth.RequireBearerToken(verifyJamseshToken, nil)(handler)
  r.Method("POST",   "/mcp", authed)
  r.Method("GET",    "/mcp", authed)
  r.Method("DELETE", "/mcp", authed)
  ```
  Reasons over alternatives: (1) v1.x stable typed-generics tool registration
  vs. mark3labs/mcp-go's 0.x; (2) `auth.RequireBearerToken` middleware emits
  RFC 9728-compliant 401s for free — inline auth in `getServer` only yields
  an opaque HTTP 400; (3) first-party so spec changes land fast. v1.6.0
  defaults to MCP protocol `2025-11-25` and back-negotiates to `2025-06-18` /
  `2025-03-26` automatically. Fallback only on a v1.6 bug: `mark3labs/mcp-go`
  is a safe second choice. **Cross-origin protection is OFF by default in
  v1.6.0** — wrap with `http.NewCrossOriginProtection().Handler(h)` if any
  browser-facing surface ever consumes the MCP endpoint directly.

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

Locked at epic-design time (this pass):

- **Event log persistence**: DB-persistent, indefinite per-session retention
  until session archival (per SPEC.md's 90-day post-end window). On archival
  the events rows are deleted with the rest of session data. Rationale:
  simplest model; matches the data layer; restore semantics per
  PRINCIPLES.md.
- **WebSocket auth at upgrade time**: subprotocol-encoded token —
  client sends `Sec-WebSocket-Protocol: jamsesh.bearer.<token>` in the
  upgrade request. Server validates via the foundation tokens helper,
  checks session membership, then accepts the upgrade. Rationale:
  browser WebSocket API doesn't allow custom Authorization headers;
  subprotocol avoids tokens in URLs (no leaks to access logs/history).
- **WebSocket subscription model**: per-connection per-session,
  path-based: `wss://portal/ws/sessions/<session_id>`. One connection =
  one session subscription; multi-session clients open multiple
  connections. Rationale: simplest authorization model; per-session
  fanout is cleanly bounded.
- **`query_session_state` default `include` set**:
  `[goal, scope, draft_tip, unresolved_comments_addressed_to_caller,
  open_conflicts_addressed_to_caller, recent_events_since_last_call]`.
  Addressed-to-caller filters keep the default response useful to the
  caller without noise. Rationale: matches the "what an agent typically
  needs without specifying" intent of the escape hatch.
- **Cursor format**: opaque base64 of `(filter_hash, last_seq_id)`.
  When the filter changes between calls, cursor's filter_hash mismatches
  and the server returns a `pagination.cursor_filter_mismatch` error;
  clients reset by sending no cursor. Rationale: prevents subtle
  correctness bugs from stale-cursor reuse under filter change.
- **No server-side comment dedup**: clients are responsible for
  idempotency. Posting the same comment twice produces two rows.
  Rationale: dedup is policy, not infrastructure; deferred until
  concrete user pain.
- **Bare repo creation cross-epic call shape**: direct Go function call.
  `POST /api/sessions` handler imports `epic-portal-git-storage`'s
  package and calls its init helper. No interface boundary, no RPC.
  Rationale: aligns with single-binary deployment from SPEC.md; no
  premature abstraction.

## Decomposition

Five child features around the shared event log. `events-log` is the
foundation that the other four consume — WebSocket fan-out reads from
it, REST endpoints emit into it, MCP tools proxy to REST library
functions that emit. The other four parallelize after events-log lands;
`mcp-endpoint` is the assembly point because the four MCP tools each
need at least one of the REST features as a thin-proxy target.

Critical path: `events-log → {websocket-gateway || sessions-rest ||
comments-rest} → mcp-endpoint`. Four deep with three-way parallel in
the middle band. Heavy cross-epic dependencies: sessions-rest pulls
from `foundation-accounts`, `foundation-auth-flows`, `git-storage`;
mcp-endpoint pulls from `git-storage` for the fork tool.

### Child features

- `epic-portal-api-events-log` — events + presence tables, monotonic
  per-session seq, emit helpers (single + batch + presence-update),
  envelope shape lock — depends on:
  `[epic-portal-foundation-data-layer]`
- `epic-portal-api-websocket-gateway` — `GET /ws/sessions/<id>` with
  subprotocol-token auth, in-memory per-session subscription registry,
  fan-out, replay-from-cursor, heartbeat — depends on:
  `[epic-portal-api-events-log, epic-portal-foundation-http-skeleton,
  epic-portal-foundation-tokens]`
- `epic-portal-api-sessions-rest` — sessions CRUD (create/list/get/
  patch/finalize/abandon), refs listing, digest endpoint, invites +
  member management — depends on:
  `[epic-portal-api-events-log, epic-portal-foundation-http-skeleton,
  epic-portal-foundation-accounts, epic-portal-foundation-auth-flows,
  epic-portal-git-storage]`
- `epic-portal-api-comments-rest` — comments table, conflict_events
  table read API, list + resolve endpoints, internal library functions
  consumed by the MCP `post_comment` / `resolve_comment` tools — depends
  on: `[epic-portal-api-events-log,
  epic-portal-foundation-http-skeleton]`
- `epic-portal-api-mcp-endpoint` — streamable-http transport mount,
  per-request Bearer auth via the SDK's `getServer` callback, the four
  thin-proxy tool implementations — depends on:
  `[epic-portal-api-events-log, epic-portal-api-sessions-rest,
  epic-portal-api-comments-rest, epic-portal-foundation-tokens,
  epic-portal-foundation-http-skeleton, epic-portal-git-storage]`

### Decomposition risks

- **`sessions-rest` is at the size ceiling.** 12-15 implementation
  units. If the design pass surfaces additional complexity around
  invite-acceptance edge cases or finalize lock semantics, the design
  pass may signal back to autopilot to split out a
  `sessions-membership` feature. Capacity reserved.
- **WebSocket gateway is the highest-risk feature.** Replay-from-cursor
  with bounded retention, per-session fanout fairness, backpressure
  under slow consumers — all subtle. Design pass produces an explicit
  lifecycle diagram and a replay/backpressure test plan.
- **MCP SDK rough-edge risk.** The locked v1.x of
  `modelcontextprotocol/go-sdk` is recent. Design pass on
  `mcp-endpoint` starts with a spike — wire `query_session_state`
  end-to-end (simplest of the four), confirm auth + dispatch work, THEN
  design the other three.
- **Event log growth is unbounded per active session.** Long-running
  sessions accumulate events. Design pass on events-log produces a
  per-session row-count metric so growth is observable; archival
  (events older than N days → cold table) is a documented follow-up if
  growth becomes a problem.

## Final review (2026-05-17)

**Verdict**: Approve

**Notes**: All 5 child features done: events-log (typed event envelope + monotonic seq), sessions-rest (11 endpoints, full session lifecycle + invites), comments-rest (Service + REST), websocket-gateway (real-time push), mcp-endpoint (4 tools the CC plugin consumes). The portal API surface is complete end-to-end.
