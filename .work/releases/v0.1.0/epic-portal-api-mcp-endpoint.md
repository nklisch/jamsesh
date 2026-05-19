---
id: epic-portal-api-mcp-endpoint
kind: feature
stage: done
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-api-sessions-rest, epic-portal-api-comments-rest, epic-portal-foundation-tokens, epic-portal-foundation-http-skeleton, epic-portal-git-storage]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
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

## Design decisions

- **SDK**: `github.com/modelcontextprotocol/go-sdk` v1.x per the auto-loaded `mcp-go-sdk` skill.
- **Package**: `internal/portal/mcpendpoint/`.
- **Auth via `NewStreamableHTTPHandler` callback**: `getServer(r *http.Request) *mcp.Server` — extract Bearer token, validate via tokens.Service, return a per-request mcp.Server with the account captured.
- **Tool routing**: each tool closure captures `account`. session_id from tool args → membership check via Store → delegate to library functions:
  - post_comment → `comments.Service.Create`
  - resolve_comment → `comments.Service.Resolve`
  - fork → server-side ref creation (this feature owns the impl since no other feature has it)
  - query_session_state → assembles from sessions.Service + comments.Service + events.Log
- **ref_modes for fork**: reuse the `ref_modes` table from sessions-rest. fork upserts the row.
- **Single story**: cohesive feature.

## Implementation Units

### Unit 1: Scaffold + auth callback

**File**: `internal/portal/mcpendpoint/handler.go`
**Story**: `epic-portal-api-mcp-endpoint-scaffold-and-tools`

```go
package mcpendpoint

import (
    "context"
    "net/http"

    "github.com/modelcontextprotocol/go-sdk/mcp"

    "jamsesh/internal/db/store"
    "jamsesh/internal/portal/comments"
    "jamsesh/internal/portal/events"
    "jamsesh/internal/portal/storage"
    "jamsesh/internal/portal/tokens"
)

type Endpoint struct {
    Store          store.Store
    Tokens         tokens.Service
    Storage        storage.Service
    Log            *events.Log
    Comments       *comments.Service
}

func (e *Endpoint) Handler() http.Handler {
    return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
        // Auth
        authz := r.Header.Get("Authorization")
        const prefix = "Bearer "
        if !strings.HasPrefix(authz, prefix) { return nil }
        token := strings.TrimPrefix(authz, prefix)
        account, err := e.Tokens.Validate(r.Context(), token)
        if err != nil { return nil }
        
        // Build per-request server with tools capturing account
        s := mcp.NewServer(&mcp.Implementation{Name: "jamsesh", Version: "0.1"}, nil)
        e.registerTools(s, account)
        return s
    }, nil)
}

func (e *Endpoint) registerTools(s *mcp.Server, account *store.Account) {
    // 4 tools
}
```

### Unit 2-5: Tool implementations

Per parent feature body. Each tool:
1. Decode params (typed struct)
2. Verify session membership via `GetSessionMember(orgID, sessionID, account.ID)` — but the tool only has sessionID. The orgID comes from listing memberships: walk `ListSessionMembershipsForAccount(account.ID)`, find matching sessionID, extract orgID; if not found → permission error
3. Delegate to library:
   - post_comment → `Comments.Create(ctx, params)`
   - resolve_comment → `Comments.Resolve(ctx, ...)`
   - fork → create ref under `refs/heads/jam/<sessionID>/<account.ID>/<branch>` pointing at target_commit; upsert ref_modes; emit `ref.forked` event
   - query_session_state → assemble {goal, scope, draft_tip, unresolved_comments, open_conflicts, recent_events} per default includes

### fork implementation

```go
func (e *Endpoint) doFork(ctx, account *store.Account, orgID, sessionID, targetCommit, targetRef string, mode string) (forkResult, error) {
    repo, err := openRepo(e.Storage, orgID, sessionID)
    if err != nil { return ... }
    
    // Verify target commit exists
    targetHash := plumbing.NewHash(targetCommit)
    if _, err := repo.CommitObject(targetHash); err != nil { return ..., "target commit not found" }
    
    // Compute ref name: refs/heads/jam/<sessionID>/<account.ID>/<branch>
    branch := targetRef
    if branch == "" { branch = "fork-" + targetCommit[:7] }
    refName := plumbing.NewBranchReferenceName(fmt.Sprintf("jam/%s/%s/%s", sessionID, account.ID, branch))
    
    // Create ref
    ref := plumbing.NewHashReference(refName, targetHash)
    if err := repo.Storer.SetReference(ref); err != nil { return ... }
    
    // Upsert mode
    if mode == "" { mode = "sync" }  // default
    e.Store.UpsertRefMode(ctx, store.UpsertRefModeParams{SessionID: sessionID, Ref: refName.String(), Mode: mode})
    
    // Emit ref.forked event
    payload := openapi.RefForkedPayload{ParentSha: targetCommit, NewRef: refName.String(), Mode: mode}
    data, _ := json.Marshal(payload)
    e.Log.Emit(ctx, orgID, sessionID, "ref.forked", data)
    
    return forkResult{Ref: refName.String(), Sha: targetCommit}, nil
}
```

### query_session_state

Aggregate:
- session (sessions.Service.Get)
- unresolved comments addressed_to caller (comments.Service.List with addressed_to filter = account.Email)
- open conflict events addressed_to caller (Store.ListOpenConflictEventsForSession + filter by addressed_to)
- recent events since since_seq (events.Log.ListSince)

Return as a typed struct.

## Implementation Order

Single story.

## go.mod additions

- `github.com/modelcontextprotocol/go-sdk@latest`

## Testing

- httptest server + mcp client (the SDK provides one) — call each tool
- Auth: bad token → SDK returns 401
- Non-member session → permission error from tool
- Each tool's happy path
- query_session_state: assembled response shape

## Risks

- **SDK API churn**: the SDK is pre-1.0; specific function signatures may shift. The `mcp-go-sdk` skill carries the verified API surface — use it as canonical.

## Implementation summary

Single child story done. The 4 MCP tools (post_comment, resolve_comment, fork, query_session_state) are live; CC plugin can now invoke them end-to-end with token auth.

## Review

**Verdict**: Approve. portal-api epic has all 5 children done (events-log, sessions-rest, comments-rest, websocket-gateway, mcp-endpoint).
