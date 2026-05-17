# Research: MCP Go SDK (`modelcontextprotocol/go-sdk`)

**Date:** 2026-05-16
**Author:** research pass
**Status:** locked — verified for v1.6.0

## Context

The jamsesh portal exposes 4 MCP tools — `post_comment`, `resolve_comment`,
`fork`, `query_session_state` — to multi-agent Claude Code sessions over
HTTPS-MCP (`streamable-http` transport). The endpoint mounts at `/mcp` in
a chi v5 router and shares the portal's user-OAuth Bearer auth model with
the REST API.

The epic (`epic-portal-api`) locks the official
`github.com/modelcontextprotocol/go-sdk` v1.6.0+ and flags "rough-edge
risk" because the SDK is recent. This research verifies the current API
surface, integration pattern, and known pitfalls so the design pass on
`epic-portal-api-mcp-endpoint` can start with a correct spike.

## Questions

1. What is current stable? (epic locked v1.6.0+ — verify)
2. Exact `NewStreamableHTTPHandler` signature & `getServer` contract
3. How does typed-generic `AddTool` work for our 4 tools?
4. Auth plumbing: HTTP request -> tool handler with user identity
5. MCP spec version targeted
6. HTTP method/endpoint/header shape of streamable-http
7. `mark3labs/mcp-go` fallback viability

## Options Evaluated

### Official `modelcontextprotocol/go-sdk` (LOCKED)

- **Current:** v1.6.0 (2026-05-08). 24 releases. Maintained by
  Anthropic + Google.
- **API:** v1.x stable; typed generics for tools; first-party so spec
  changes land fast.
- **Spec target:** MCP **2025-11-25** (latest spec) — supports 2025-06-18
  and 2025-03-26 for back-compat. Note the epic mentions "2025-06-18 /
  November 2025 release line" — the SDK at v1.6 actually defaults to
  2025-11-25 as `latestProtocolVersion`. Both are negotiated at
  `initialize` time; either side may downgrade.
- **Go requirement:** Go 1.25+ (v1.4.1+).

### `mark3labs/mcp-go` (FALLBACK ONLY)

- v0.54.0 (2026-05-13). Still 0.x — API not stable. Active and
  feature-complete (OAuth via RFC 9728, streamable-http, CORS).
- Use only if a hard v1.6 bug blocks us. API will require rewrite at the
  ServerOptions/handler layer.

## Recommendation

Stay with `github.com/modelcontextprotocol/go-sdk @ v1.6.0`. **Pin the
exact version** in `go.mod` — minor versions in this SDK still ship
breaking changes (v1.6.0: SetError preserves existing Content; cross-
origin protection off by default; v1.4.1: Go 1.25 bump). Re-evaluate
upgrades manually, gated on release notes.

The auth pattern in the locked epic sketch (`if !validBearer ... return
nil`) **works** (nil from `getServer` -> HTTP 400) but is **not the
canonical pattern**. The SDK ships `auth.RequireBearerToken` middleware
which wraps the handler, returns proper 401 with `WWW-Authenticate`
header per RFC 9728, and stashes `*auth.TokenInfo` in `context.Context`.
Tool handlers retrieve identity via `auth.TokenInfoFromContext(ctx)`.

**Divergence from epic:** swap `getServer`-based rejection for the
`auth.RequireBearerToken` middleware wrap. The `getServer` callback
becomes trivial (just `return server`) and a verifier function does the
token validation. Per-request `*Server` instances are still possible
inside `getServer` but the canonical pattern uses one shared server.

## Implementation Notes

- **Handler is per-handler-instance, sessions live inside it.** One
  `*StreamableHTTPHandler` owns a `sessions map[string]*sessionInfo`
  keyed by `Mcp-Session-Id`. `getServer` is called for new sessions
  (or every request in `Stateless: true` mode).
- **Session hijacking protection:** when `TokenInfo.UserID` is non-
  empty, subsequent requests for the same `Mcp-Session-Id` must carry
  the same user ID; mismatch -> 403.
- **Cross-origin protection:** **OFF by default in v1.6.0** (regression
  from v1.4.1/v1.5.0). For browser-exposed endpoints, wrap explicitly:
  `protection := http.NewCrossOriginProtection(); h := protection.Handler(mcpHandler)`.
  jamsesh's `/mcp` endpoint is for Claude Code clients (not browsers),
  so this is low priority but worth setting.
- **DNS-rebinding protection:** ON by default since v1.4.1 — localhost-
  bound listeners reject non-localhost `Host` headers. jamsesh portal
  listens on the public host so this is a no-op in prod; matters if a
  dev runs the binary on 127.0.0.1.
- **Tool input schemas** are auto-inferred from the `In` type parameter
  of `AddTool[In, Out]` using `github.com/google/jsonschema-go` (2020-12
  draft only). Use `jsonschema:"description here"` struct tags for
  per-field descriptions. Override by setting `Tool.InputSchema`
  explicitly.
- **`session_id` cross-cutting field:** define it as a required field on
  every tool's input struct (`SessionID string \`json:"session_id"
  jsonschema:"jamsesh session id (uuid)"\``). The SDK validates the
  arguments against the inferred schema before the handler runs, so
  missing/empty session_id fails fast.
- **`*http.Request` access:** tool handlers receive `*mcp.CallToolRequest`
  whose `.Extra` field (`*RequestExtra`) has `Header http.Header` and
  `TokenInfo *auth.TokenInfo`. Prefer `auth.TokenInfoFromContext(ctx)`
  for identity; use `req.Extra.Header` if you need other headers.
- **Stateful vs Stateless:** default is stateful (sessions persist
  across POST/GET/DELETE on `Mcp-Session-Id`). jamsesh should use
  stateful — agents reuse sessions across multi-turn calls and may want
  SSE for progress events.

## Code Examples

### Full chi-mounted streamable-http handler with Bearer auth

```go
package portal

import (
    "context"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/modelcontextprotocol/go-sdk/auth"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// verifyToken validates the bearer token against the foundation tokens
// helper and returns a TokenInfo with UserID populated so the SDK can
// enforce session-hijacking protection.
func (p *Portal) verifyToken(
    ctx context.Context, token string, _ *http.Request,
) (*auth.TokenInfo, error) {
    user, err := p.tokens.Validate(ctx, token)
    if err != nil {
        return nil, auth.ErrInvalidToken
    }
    return &auth.TokenInfo{
        UserID:     user.ID,
        Scopes:     []string{"mcp"},
        Expiration: user.TokenExpiresAt,
    }, nil
}

func (p *Portal) mountMCP(r chi.Router) {
    // One server instance, shared across all sessions.
    server := mcp.NewServer(&mcp.Implementation{
        Name:    "jamsesh",
        Version: p.version,
    }, nil)

    // Register the 4 tools (typed generics).
    mcp.AddTool(server, &mcp.Tool{
        Name:        "post_comment",
        Description: "Post a review comment on a draft commit or scope.",
    }, p.postComment)
    mcp.AddTool(server, &mcp.Tool{
        Name:        "resolve_comment",
        Description: "Resolve a previously-posted comment.",
    }, p.resolveComment)
    mcp.AddTool(server, &mcp.Tool{
        Name:        "fork",
        Description: "Fork a ref into a new branch for parallel exploration.",
    }, p.fork)
    mcp.AddTool(server, &mcp.Tool{
        Name:        "query_session_state",
        Description: "Query current session state (goal, scope, open conflicts).",
    }, p.querySessionState)

    handler := mcp.NewStreamableHTTPHandler(
        func(_ *http.Request) *mcp.Server { return server },
        &mcp.StreamableHTTPOptions{
            Logger:         p.logger,
            SessionTimeout: 30 * time.Minute,
        },
    )

    authMW := auth.RequireBearerToken(p.verifyToken, &auth.RequireBearerTokenOptions{
        Scopes: []string{"mcp"},
    })

    r.Method(http.MethodPost,   "/mcp", authMW(handler))
    r.Method(http.MethodGet,    "/mcp", authMW(handler))
    r.Method(http.MethodDelete, "/mcp", authMW(handler))
}
```

### One typed-tool registration — `post_comment`

```go
type PostCommentInput struct {
    SessionID string `json:"session_id" jsonschema:"jamsesh session id (uuid)"`
    Target    string `json:"target"     jsonschema:"comment target (e.g. ref:scope, ref:<sha>)"`
    Body      string `json:"body"       jsonschema:"comment body markdown"`
    AddressTo string `json:"address_to,omitempty" jsonschema:"agent id to address (optional)"`
}

type PostCommentOutput struct {
    CommentID string `json:"comment_id"`
    Seq       int64  `json:"seq"`
}

func (p *Portal) postComment(
    ctx context.Context,
    _ *mcp.CallToolRequest,
    in PostCommentInput,
) (*mcp.CallToolResult, PostCommentOutput, error) {
    info := auth.TokenInfoFromContext(ctx)
    if info == nil {
        return nil, PostCommentOutput{}, fmt.Errorf("unauthenticated")
    }
    // Session-scoped authorization: caller must be a member.
    if err := p.sessions.AuthorizeMember(ctx, in.SessionID, info.UserID); err != nil {
        return nil, PostCommentOutput{}, err
    }
    // Thin proxy into the comments REST library.
    c, err := p.comments.Post(ctx, in.SessionID, info.UserID, comments.Input{
        Target: in.Target, Body: in.Body, AddressTo: in.AddressTo,
    })
    if err != nil {
        return nil, PostCommentOutput{}, err
    }
    return nil, PostCommentOutput{CommentID: c.ID, Seq: c.Seq}, nil
}
```

The SDK auto-populates the textual `Content` from the marshaled
`PostCommentOutput`, so returning `nil` for the `*mcp.CallToolResult` is
fine. Errors are packed into `CallToolResult.Content` with `IsError:
true` — they are tool errors, not protocol errors.

## Streamable-HTTP transport shape

- **Endpoints:** all on the mounted path (`/mcp`).
- **Methods:**
  - `POST` — send JSON-RPC messages (requests, responses, notifications);
    may create a session
  - `GET` — open standalone SSE stream, or resume an interrupted stream
  - `DELETE` — terminate the session
- **Required headers:**
  - `Accept: application/json, text/event-stream` (both)
  - `Content-Type: application/json` (POST)
  - `Mcp-Session-Id: <id>` (GET/DELETE, and POST after first response)
  - `Mcp-Protocol-Version: 2025-11-25` (after negotiation)
- **Response formats:**
  - `text/event-stream` (default) — supports streaming + server-initiated
    messages
  - `application/json` (set `StreamableHTTPOptions.JSONResponse: true`) —
    single JSON response, no streaming

## Known pitfalls / rough edges (v1.x)

1. **Cross-origin protection regression in v1.6.0.** OFF by default;
   was ON in v1.4.1/v1.5.0. Compatibility flag `enableoriginverification=1`
   via `GODEBUG`-style env (`MCPGODEBUG`) restores it; removal in v1.8.0.
   Wrap with `http.NewCrossOriginProtection().Handler(...)` for browser-
   exposed endpoints.
2. **`SetError` semantics change in v1.6.0.** Now preserves existing
   Content if populated. Old behavior via `MCPGODEBUG=seterroroverwrite=1`.
3. **Schema draft pinned to 2020-12.** Custom `InputSchema` outside this
   draft is rejected at registration. Use `jsonschema-go` for hand-rolled
   schemas.
4. **Go 1.25 minimum** since v1.4.1.
5. **GetSessionID moved onto `ServerOptions`** in a prior breaking
   change. If migrating from <v1.4, watch for the API move.
6. **Keepalive ping bug fixed only in late v1.5/v1.6.** Earlier versions
   silently killed sessions when peer didn't implement `ping`.
7. **`getServer` returns nil -> HTTP 400.** Usable as an auth-reject
   path but the error body is `"no server available"` — opaque to
   clients. Prefer `auth.RequireBearerToken` middleware which returns
   proper 401 + `WWW-Authenticate: Bearer resource_metadata=...`.
8. **MCPGODEBUG env-based compatibility flags** are temporary — most
   are slated for removal in v1.8.0 or v1.9.0. Don't rely on them.
9. **No client-disconnect callback on `StreamableHTTPHandler`.**
   `SessionTimeout` is the only built-in cleanup hook (closes idle
   sessions after duration).

## References

- Repo: <https://github.com/modelcontextprotocol/go-sdk>
- Release v1.6.0 (2026-05-08): <https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.6.0>
- Package docs: <https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp>
- HTTP example: <https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/http/main.go>
- Auth example: <https://github.com/modelcontextprotocol/go-sdk/blob/main/examples/auth/server/main.go>
- MCP spec 2025-11-25: <https://modelcontextprotocol.io/specification/2025-11-25>
- `auth` package: <https://github.com/modelcontextprotocol/go-sdk/blob/main/auth/auth.go>
- Streamable HTTP transport: <https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go>
- Fallback: <https://github.com/mark3labs/mcp-go> (v0.54.0)
- Epic: `.work/active/epics/epic-portal-api.md`
