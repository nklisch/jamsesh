---
id: epic-portal-api-mcp-endpoint
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-api-sessions-rest, epic-portal-api-comments-rest, epic-portal-foundation-tokens, epic-portal-foundation-http-skeleton, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal API — MCP Endpoint

## Brief

The HTTPS-MCP endpoint (`streamable-http` transport) that Claude Code
clients connect to. Exposes the four jamsesh tools, each authenticated
per-request via the foundation tokens helper and authorized per-session
via the `session_id` argument every tool call carries.

**Transport**: streamable-http per the MCP spec, mounted at `/mcp` on
the portal HTTP server (route owned by http-skeleton). Connection
upgrade and message dispatch are handled by the
`modelcontextprotocol/go-sdk` (locked at epic-design — v1.x).

**Auth wiring** (the cleanest part of the locked SDK choice):

```go
handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    accountID, ok := tokens.ValidateBearer(r.Header.Get("Authorization"))
    if !ok { return nil }  // 401 from the SDK
    s := mcp.NewServer(&mcp.Implementation{Name: "jamsesh", Version: "0.1"}, nil)
    mcp.AddTool(s, &mcp.Tool{Name: "post_comment", ...}, postComment(accountID))
    mcp.AddTool(s, &mcp.Tool{Name: "resolve_comment", ...}, resolveComment(accountID))
    mcp.AddTool(s, &mcp.Tool{Name: "fork", ...}, fork(accountID))
    mcp.AddTool(s, &mcp.Tool{Name: "query_session_state", ...}, queryState(accountID))
    return s
}, nil)
mux.Mount("/mcp", handler)
```

Each tool closure captures the authenticated `accountID`; the
`session_id` parameter from the call drives the session-membership
check before delegating to the library functions exported from
`sessions-rest`, `comments-rest`, and `events-log`.

**Tool implementations** (each a thin proxy):

- `post_comment` — `session_id`, `commit_sha`, optional `file_path`,
  optional `line_range`, `body`, optional `addressed_to`, optional
  `kind`. Delegates to `comments-rest`'s `CreateComment`.
- `resolve_comment` — `session_id`, `comment_id`, optional
  `resolution_note`. Delegates to `comments-rest`'s `ResolveComment`.
- `fork` — `session_id`, `target_commit_sha`, optional `target_ref`,
  optional `mode`. Server-side ref manipulation: validates the target
  commit exists in the session bare repo (via `epic-portal-git-storage`
  + go-git), creates or moves the ref under
  `jam/<session>/<account>/...`, sets the mode in a `ref_metadata` table
  (owned by sessions-rest or here — design pass picks), emits
  `ref.forked` event. Returns `{ref, sha}`.
- `query_session_state` — `session_id`, optional `include[]`, optional
  `filter`. Returns an object keyed by requested includes. Default
  include set (locked at epic-design):
  `[goal, scope, draft_tip, unresolved_comments_addressed_to_caller,
  open_conflicts_addressed_to_caller, recent_events_since_last_call]`.

**Tool routing pattern**: tools are thin proxies; the substantive
behavior lives in the corresponding REST feature's exported library
functions. This keeps the MCP and REST surfaces in lock-step semantics
without code duplication.

Does NOT include the SDK's streamable-http transport itself (consumes
it). Does NOT cover the auth-flow OAuth + magic-link surface (foundation
auth-flows). Does NOT cover the actual comment/session state mutations
(delegated to comments-rest / sessions-rest).

## Epic context

- Parent epic: `epic-portal-api`
- Position in epic: assembly point — depends on every other feature in
  this epic (events-log, sessions-rest, comments-rest) plus the
  foundation tokens helper and the cross-epic
  `epic-portal-git-storage` (for fork's bare-repo ref manipulation).

## Foundation references

- `docs/PROTOCOL.md` — MCP tools (all four signatures, parameter and
  return shapes are the canonical contract)
- `docs/ARCHITECTURE.md` — MCP endpoint subcomponent, Data flow
- `docs/SECURITY.md` — MCP authorization (Bearer + session-scoped check)
- `docs/SPEC.md` — Stack > Backend (MCP endpoint), Auth model

## Inherited epic design decisions

- **MCP SDK**: `github.com/modelcontextprotocol/go-sdk` v1.x. Drop-in
  chi mount via `NewStreamableHTTPHandler` with the `getServer(*http.Request)`
  callback for per-request auth.
- **Tool routing pattern**: thin-proxy. Tools delegate to library
  functions exported from the REST features. MCP and REST stay in
  semantic lock-step.
- **`query_session_state` defaults**: addressed-to-caller filters in
  the default include set.

## Decomposition risks

- Second-highest risk in this epic. The SDK lock at v1.x is recent
  enough that the streamable-http transport + `getServer` callback +
  typed-struct tool registration combo may have rough edges in
  practice. Mitigation: design pass starts with a spike — wire one
  tool (`query_session_state` is simplest) end-to-end with the SDK,
  confirm auth and dispatch work, THEN design the other three.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
