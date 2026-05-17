---
name: mcp-go-sdk
description: Reference for the official MCP Go SDK (github.com/modelcontextprotocol/go-sdk). Auto-loads when editing Go files that import github.com/modelcontextprotocol/go-sdk/mcp or .../auth, when wiring jamsesh's /mcp endpoint, or when implementing any of the four MCP tools — post_comment, resolve_comment, fork, query_session_state. Also triggers on terms — mcp.NewServer, mcp.AddTool, mcp.NewStreamableHTTPHandler, mcp.StreamableHTTPHandler, mcp.StreamableHTTPOptions, mcp.Implementation, mcp.Tool, mcp.CallToolRequest, mcp.CallToolResult, mcp.ToolHandlerFor, mcp.RequestExtra, mcp.ServerSession, auth.RequireBearerToken, auth.TokenInfo, auth.TokenInfoFromContext, auth.ErrInvalidToken, streamable-http, MCP server, MCP tool, Mcp-Session-Id, modelcontextprotocol.
user-invocable: false
---

# MCP Go SDK reference (jamsesh)

**Pinned version**: v1.6.0 (2026-05-08). Module:
`github.com/modelcontextprotocol/go-sdk`. Sub-packages used:
`mcp`, `auth`. **Go 1.25+ required.**

MCP protocol spec target: **2025-11-25** (negotiates back to 2025-06-18
and 2025-03-26 at `initialize`).

## Why this SDK here (locked decision)

Official Anthropic+Google SDK. v1.x API stable; typed generics on
`AddTool` give compile-time-checked tool registration. Fallback if a
hard v1.6 bug surfaces: `github.com/mark3labs/mcp-go` (v0.x, API
unstable but mature).

## Canonical chi-mounted handler with Bearer auth

This is **the** pattern. Do NOT do auth inside `getServer` — the locked
epic sketch shows that but it produces opaque 400s. Use the SDK's
`auth.RequireBearerToken` middleware wrap (proper 401 + RFC 9728
`WWW-Authenticate`):

```go
import (
    "context"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/modelcontextprotocol/go-sdk/auth"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (p *Portal) verifyToken(
    ctx context.Context, token string, _ *http.Request,
) (*auth.TokenInfo, error) {
    user, err := p.tokens.Validate(ctx, token)
    if err != nil {
        return nil, auth.ErrInvalidToken
    }
    return &auth.TokenInfo{
        UserID:     user.ID,        // enables SDK session-hijacking protection
        Scopes:     []string{"mcp"},
        Expiration: user.TokenExpiresAt, // MUST be non-zero or 401
    }, nil
}

func (p *Portal) mountMCP(r chi.Router) {
    server := mcp.NewServer(&mcp.Implementation{
        Name: "jamsesh", Version: p.version,
    }, nil)

    mcp.AddTool(server, &mcp.Tool{Name: "post_comment",        Description: "..."}, p.postComment)
    mcp.AddTool(server, &mcp.Tool{Name: "resolve_comment",     Description: "..."}, p.resolveComment)
    mcp.AddTool(server, &mcp.Tool{Name: "fork",                Description: "..."}, p.fork)
    mcp.AddTool(server, &mcp.Tool{Name: "query_session_state", Description: "..."}, p.querySessionState)

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

    // Streamable-http uses POST, GET, DELETE on the same path.
    r.Method(http.MethodPost,   "/mcp", authMW(handler))
    r.Method(http.MethodGet,    "/mcp", authMW(handler))
    r.Method(http.MethodDelete, "/mcp", authMW(handler))
}
```

## Typed tool registration — worked example

Every jamsesh tool input struct **must** include `SessionID` as a
required JSON-schema field. The SDK validates against the inferred
schema before the handler fires, so missing `session_id` fails fast.

```go
type PostCommentInput struct {
    SessionID string `json:"session_id" jsonschema:"jamsesh session id (uuid)"`
    Target    string `json:"target"     jsonschema:"comment target (ref:scope, ref:<sha>, ...)"`
    Body      string `json:"body"       jsonschema:"comment body markdown"`
    AddressTo string `json:"address_to,omitempty" jsonschema:"agent id to address (optional)"`
}

type PostCommentOutput struct {
    CommentID string `json:"comment_id"`
    Seq       int64  `json:"seq"`
}

func (p *Portal) postComment(
    ctx context.Context, _ *mcp.CallToolRequest, in PostCommentInput,
) (*mcp.CallToolResult, PostCommentOutput, error) {
    info := auth.TokenInfoFromContext(ctx) // user identity from middleware
    if info == nil {
        return nil, PostCommentOutput{}, fmt.Errorf("unauthenticated")
    }
    if err := p.sessions.AuthorizeMember(ctx, in.SessionID, info.UserID); err != nil {
        return nil, PostCommentOutput{}, err // session-scoped authz
    }
    c, err := p.comments.Post(ctx, in.SessionID, info.UserID, comments.Input{
        Target: in.Target, Body: in.Body, AddressTo: in.AddressTo,
    })
    if err != nil {
        return nil, PostCommentOutput{}, err
    }
    return nil, PostCommentOutput{CommentID: c.ID, Seq: c.Seq}, nil
}
```

Returning `nil` for `*mcp.CallToolResult` is fine — the SDK marshals
the typed `Out` value into `Content` automatically. Returned errors
become tool errors (`IsError: true`), not protocol errors.

## Key API at a glance

```go
func NewServer(impl *Implementation, opts *ServerOptions) *Server
func NewStreamableHTTPHandler(
    getServer func(*http.Request) *Server,  // nil return -> HTTP 400
    opts      *StreamableHTTPOptions,
) *StreamableHTTPHandler
func AddTool[In, Out any](s *Server, t *Tool, h ToolHandlerFor[In, Out])

type ToolHandlerFor[In, Out any] func(
    ctx context.Context, request *CallToolRequest, input In,
) (result *CallToolResult, output Out, _ error)

func auth.RequireBearerToken(
    verifier TokenVerifier, opts *RequireBearerTokenOptions,
) func(http.Handler) http.Handler
func auth.TokenInfoFromContext(ctx context.Context) *TokenInfo
```

Full type definitions, `StreamableHTTPOptions`/`ServerOptions` fields,
and request/response types are in `references/api-signatures.md`.

## Streamable-HTTP transport

- All methods mount on the same path (`/mcp` in jamsesh).
  - `POST` — JSON-RPC messages; may create or use a session
  - `GET`  — open / resume standalone SSE stream
  - `DELETE` — terminate session
- Required headers: `Accept: application/json, text/event-stream`;
  `Content-Type: application/json` (POST); `Mcp-Session-Id: <id>`
  (after first response); `Mcp-Protocol-Version: 2025-11-25` (after
  negotiation).
- Default response is `text/event-stream`; set `JSONResponse: true` for
  plain JSON (loses server-initiated streaming).

## Pitfalls — quick hits

Full list with workarounds in `references/pitfalls.md`. Top items:

1. **Cross-origin protection OFF by default in v1.6.0** (was ON in
   v1.5.0). Wrap externally for browser-facing endpoints.
2. **`TokenInfo.Expiration` MUST be non-zero** or middleware returns 401.
3. **Always set `TokenInfo.UserID`** — enables session-hijacking
   protection (same session id from different user -> 403).
4. **Don't auth inside `getServer`** — opaque 400. Use the middleware.
5. **Schema draft locked to 2020-12.** Use `jsonschema:"description"`
   struct tags; custom schemas outside this draft are rejected.
6. **Pin the exact version in `go.mod`.** Minor versions still ship
   behavior changes.
7. **Go 1.25 minimum** since v1.4.1.

## Jamsesh-specific design

- One shared `*mcp.Server` instance at portal boot, not per session.
- All four tools share an input-struct `SessionID` field; session-scoped
  authorization (`p.sessions.AuthorizeMember`) runs in every handler
  before delegating to the REST library layer.
- Identity flows from `auth.RequireBearerToken` middleware -> context
  -> `auth.TokenInfoFromContext(ctx)` inside handlers. Do NOT read
  `req.Extra.Header` for the bearer — middleware already extracted it.
- MCP tools are **thin proxies** to the REST library layer. All
  business logic lives in REST; MCP handlers do auth + delegation only.
- Per `epic-portal-api`: spike `query_session_state` first end-to-end
  (simplest of the four), confirm auth + dispatch work, THEN wire the
  other three.

## Foundation references

- Epic: `.work/active/epics/epic-portal-api.md` (decisions section)
- Spec: `docs/PROTOCOL.md` (MCP tool contracts)
- Auth: `docs/SECURITY.md` (Bearer token model)
- Research: `docs/research/mcp-go-sdk.md` (full options + version notes)
- Detailed API: `references/api-signatures.md`
- Full pitfalls: `references/pitfalls.md`
