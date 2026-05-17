# Core Go Server Stack Research

Reference research for the four locked technologies in jamsesh's portal binary:
sqlc, chi, coder/websocket, and oapi-codegen. The choices are LOCKED;
this document captures current API surfaces, integration patterns, and
gotchas so implementers don't repeat the verification work.

Verified: 2026-05-16.

## Context

The portal is a single Go binary that:

- Speaks REST (`/api/*`), MCP-streamable-HTTP (`/mcp`), HTTP Basic git
  smart-HTTP (`/git/*`), and WebSocket (`/ws/sessions/<id>`) over one
  TLS listener (or behind a trusted proxy).
- Persists everything in SQLite by default; Postgres swap is a hard
  requirement for the same schema.
- Uses `docs/openapi.yaml` as the **single source of truth** for the
  REST API AND the WebSocket event-envelope/payload schemas. Both Go
  server and TypeScript client regenerate from it. Drift is a compile
  error in Go; CI runs `make generate && git diff --exit-code`.
- Enforces `org_id` boundaries at the data layer — every query that
  touches an org-scoped table accepts `org_id` and includes it in
  WHERE.

Foundation references:
- `docs/SPEC.md` — Stack section, Generated contracts section
- `docs/ARCHITECTURE.md` — Portal component subcomponents
- `docs/SECURITY.md` — Authentication, Authorization
- `.work/active/epics/epic-portal-foundation.md` — design decisions
  for data-layer + http-skeleton + tokens
- `.work/active/epics/epic-portal-api.md` — design decisions for
  websocket-gateway + mcp-endpoint

## Questions

1. What is the current canonical import path and version for each
   library? (Some have been renamed/forked.)
2. What is the dual-dialect sqlc pattern that lets SQLite and Postgres
   share a single Go access layer?
3. How does chi's per-subroute middleware composition handle the
   multi-auth shape (Bearer / Basic / subprotocol-token)?
4. What is the subprotocol-token WebSocket upgrade pattern with
   coder/websocket?
5. Does oapi-codegen support OpenAPI 3.1 today? If not, what's the
   workaround?
6. What strict-server pattern does oapi-codegen offer for chi, and how
   do generated structs flow into the WebSocket gateway?

## Options Evaluated

All four are LOCKED — this section documents the verified current
state, not a re-evaluation.

### sqlc

- **Current version**: v1.31.1 (released 2026-04-22).
- **License**: MIT.
- **Maturity**: production-grade. Used by Stripe, Salesforce, Vercel,
  and others.
- **Module**: `github.com/sqlc-dev/sqlc` (CLI binary). Generated code
  has no runtime dependency on sqlc.
- **OpenAPI/3.1 relevance**: none — separate layer.
- **Dual-dialect support**: native. The `sqlc.yaml` v2 schema accepts
  multiple `sql:` blocks, each with its own `engine`, `queries`,
  `schema`, and `gen.go.out`. The result is two parallel Go packages
  with isomorphic surface (same query names, same param types) where
  schema and parameters align. The runtime selects which package to
  call by the configured driver — typically a `Store` interface that
  both `sqlitestore` and `pgstore` implement.

### chi

- **Current version**: v5.2.5 (released 2026-02-05).
- **License**: MIT.
- **Maturity**: production-grade. Used by Cloudflare, Heroku,
  Stripe, and others. Stable v5 API since 2021.
- **Module**: `github.com/go-chi/chi/v5`.
- **Middleware package**: `github.com/go-chi/chi/v5/middleware`
  (RequestID, RealIP, Logger, Recoverer, Timeout, Compress,
  CleanPath, NoCache, BasicAuth, …).
- **stdlib compatibility**: routers ARE `http.Handler`; middleware is
  `func(http.Handler) http.Handler`. Drops into any `*http.ServeMux`.

### coder/websocket (formerly nhooyr.io/websocket)

- **Current version**: v1.8.14 (released 2025-09-06). Latest stable as
  of 2026-05-16.
- **License**: ISC.
- **Maturity**: production-grade, RFC 6455 + RFC 7692 compliant,
  context-aware, zero dependencies.
- **Module**: `github.com/coder/websocket`. **Use this path.** The
  `nhooyr.io/websocket` import is still resolvable for legacy builds
  but the project moved to Coder's stewardship in 2024. Per the
  project README: Coder maintains it; nhooyr maintained it 2019-2024.
- **jamsesh epics still say `nhooyr.io/websocket`.** That's stale
  foundation-doc text — implementers should use
  `github.com/coder/websocket` and the foundation doc gets rolled
  forward the first time it's touched (rolling-foundation principle).

### oapi-codegen (post-fork canonical location)

- **Current version**: v2.7.0 (released 2026-05-01).
- **License**: Apache-2.0.
- **Maturity**: stable v2 API. Production-grade.
- **Module**: `github.com/oapi-codegen/oapi-codegen/v2`. The fork from
  `github.com/deepmap/oapi-codegen` landed in **v2.3.0 (May 2024)**.
  Anything older lives on the deepmap path; v2.3.0+ is on the
  oapi-codegen org. **Use the new path.**
- **OpenAPI 3.1 support — IMPORTANT**: the mainline v2.7.0 release
  **does not yet support OpenAPI 3.1** ("awaiting upstream support"
  per the README — kin-openapi blocker). An experimental
  `oapi-codegen-exp` repo exists with a different parser, but is
  explicitly NOT production-ready ("The generated code and command
  line options are not yet stable. Use at your own risk").

  **Workaround for jamsesh**: author `docs/openapi.yaml` against the
  3.1 spec but constrain it to the 3.0-compatible subset (no
  `webhooks` top-level, no `null` type — use `nullable: true`, no
  JSON Schema 2020-12 keywords like `prefixItems`/`if`/`then`). With
  the `openapi: 3.0.3` declaration, the spec round-trips through
  oapi-codegen v2 cleanly and through `openapi-typescript` (which
  supports 3.1 natively) without loss. Track the upstream
  kin-openapi 3.1 work and migrate the spec's declared version when
  oapi-codegen's mainline picks it up.

## Recommendations

| Tech | Import path | Pin |
|------|-------------|-----|
| sqlc | `github.com/sqlc-dev/sqlc` (CLI only) | v1.31.1 |
| chi | `github.com/go-chi/chi/v5` | v5.2.5 |
| websocket | `github.com/coder/websocket` | v1.8.14 |
| oapi-codegen | `github.com/oapi-codegen/oapi-codegen/v2` (CLI) + `github.com/oapi-codegen/runtime` | v2.7.0 |

Update `docs/SPEC.md` to reference `github.com/coder/websocket` and
`github.com/oapi-codegen/oapi-codegen/v2` the next time those sections
are touched.

## Implementation Notes

- **sqlc dialect selection**: pick at build time via a `Store`
  interface (one method per query, both sqlite and postgres packages
  implement it). Avoid runtime branching inside individual queries —
  the whole point of the dual-package layout is to keep dialect
  divergence at the call-site boundary. Where SQLite and Postgres
  accept the same SQL with the same placeholders (`?` works in
  SQLite; `$1` in Postgres), the two `.sql` files differ only in
  placeholder style. sqlc generates structurally-identical Go.

- **chi multi-auth shape**: prefer `r.Route("/api", func(r){ r.Use(BearerAuth); ... })`
  over a flat router with per-handler `With()` calls. The Route
  subrouter gets its own middleware stack — auth lives at the mount
  point, handlers stay focused on business logic. The `/mcp` mount
  uses the SDK's own `getServer(*http.Request) *mcp.Server` callback
  for per-request Bearer inspection — no chi middleware on that
  route group.

- **coder/websocket auth at upgrade time**: the browser WebSocket API
  forbids custom `Authorization` headers. jamsesh uses
  `Sec-WebSocket-Protocol: jamsesh.bearer.<token>` — server reads
  `r.Header["Sec-WebSocket-Protocol"]`, validates the token via the
  foundation's token helper, then passes `Subprotocols: []string{"jamsesh.bearer.<token>"}`
  to `websocket.Accept` so the protocol echoes back. **Pitfall**:
  the entire subprotocol string must be echoed exactly; any
  whitespace/case difference fails the handshake.

- **oapi-codegen strict-server with chi**: emit BOTH `chi-server: true`
  AND `strict-server: true`. The strict layer wraps the chi-server
  layer; handlers implement `StrictServerInterface` and receive typed
  `FooRequestObject` / return typed `FooResponseObject`. Generated
  structs (`api.Comment`, `api.Session`, etc.) are reused as the
  payload type inside the WebSocket envelope — no second source of
  truth.

- **Spec-first round-trip**: keep `docs/openapi.yaml` at
  `openapi: 3.0.3` until oapi-codegen mainline picks up 3.1. The TS
  generator handles both. CI runs `make generate && git diff --exit-code`
  to catch drift.

## Code Examples

### chi router skeleton (multi-auth)

```go
package portal

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(deps Deps) http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // REST API — Bearer auth on every route.
    r.Route("/api", func(r chi.Router) {
        r.Use(BearerAuth(deps.Tokens))
        deps.OAPIHandler(r) // mounts generated chi handlers
    })

    // Git smart-HTTP — HTTP Basic, token as password.
    r.Route("/git", func(r chi.Router) {
        r.Use(BasicAuthToken(deps.Tokens))
        r.Handle("/*", deps.GitHandler)
    })

    // MCP — Bearer inspected per-request by SDK callback.
    r.Mount("/mcp", deps.MCPHandler)

    // WebSocket — auth happens inside the handler at upgrade time.
    r.Get("/ws/sessions/{sessionID}", deps.WSHandler)

    return r
}
```

### sqlc dual-dialect query example

`db/queries/sessions.sqlite.sql`:

```sql
-- name: GetSession :one
SELECT id, org_id, name, goal, created_at
FROM sessions
WHERE org_id = ? AND id = ?;
```

`db/queries/sessions.postgres.sql`:

```sql
-- name: GetSession :one
SELECT id, org_id, name, goal, created_at
FROM sessions
WHERE org_id = $1 AND id = $2;
```

`sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: sqlite
    schema: db/schema/sqlite.sql
    queries: db/queries/sessions.sqlite.sql
    gen:
      go:
        package: sqlitestore
        out: db/sqlitestore
        emit_interface: true
        emit_json_tags: true
  - engine: postgresql
    schema: db/schema/postgres.sql
    queries: db/queries/sessions.postgres.sql
    gen:
      go:
        package: pgstore
        out: db/pgstore
        sql_package: pgx/v5
        emit_interface: true
        emit_json_tags: true
```

Runtime selection:

```go
type Store interface {
    GetSession(ctx context.Context, orgID, id string) (Session, error)
    // ... one method per query
}
```

Each generated `Querier` is wrapped by a thin adapter that satisfies
`Store`. **org_id always appears in WHERE** — enforced by code
review on every query file.

### coder/websocket subprotocol upgrade

```go
import (
    "github.com/coder/websocket"
    "github.com/coder/websocket/wsjson"
)

func (h *WSHandler) Serve(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // Subprotocol carries the bearer token.
    proto := r.Header.Get("Sec-WebSocket-Protocol")
    token, ok := strings.CutPrefix(proto, "jamsesh.bearer.")
    if !ok {
        http.Error(w, "missing subprotocol token", http.StatusUnauthorized)
        return
    }
    acct, err := h.tokens.Validate(r.Context(), token)
    if err != nil || !h.members.IsMember(r.Context(), acct.ID, sessionID) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
        Subprotocols:   []string{proto}, // echo exactly
        OriginPatterns: h.allowedOrigins,
    })
    if err != nil {
        return
    }
    defer conn.CloseNow()

    h.registry.Subscribe(sessionID, conn)
    defer h.registry.Unsubscribe(sessionID, conn)

    // CloseRead returns a context cancelled on client disconnect.
    ctx := conn.CloseRead(r.Context())
    h.fanOut(ctx, conn, sessionID)
}
```

### oapi-codegen chi strict server stub

`oapi-codegen.yaml`:

```yaml
package: api
output: internal/api/api.gen.go
generate:
  chi-server: true
  strict-server: true
  models: true
  embedded-spec: true
output-options:
  prefer-skip-optional-pointer: true
```

`go:generate` directive:

```go
//go:generate oapi-codegen -config oapi-codegen.yaml ../../docs/openapi.yaml
```

Handler implementation (strict):

```go
type Server struct {
    store Store
    bus   EventBus
}

var _ api.StrictServerInterface = (*Server)(nil)

func (s *Server) GetSession(ctx context.Context, req api.GetSessionRequestObject) (api.GetSessionResponseObject, error) {
    acct := authContext(ctx)
    sess, err := s.store.GetSession(ctx, acct.OrgID, req.SessionId)
    if errors.Is(err, sql.ErrNoRows) {
        return api.GetSession404JSONResponse{NotFoundJSONResponse: api.NotFoundJSONResponse{Code: "session.not_found"}}, nil
    }
    if err != nil {
        return nil, err
    }
    return api.GetSession200JSONResponse(toAPISession(sess)), nil
}
```

Mount on chi:

```go
r.Route("/api", func(r chi.Router) {
    r.Use(BearerAuth(deps.Tokens))
    strict := api.NewStrictHandler(server, nil)
    api.HandlerFromMux(strict, r)
})
```

The same `api.Session` struct that returns from `GetSession` is
embedded into WebSocket envelope payloads — single generated type,
zero drift.

## References

- sqlc docs: https://docs.sqlc.dev/en/latest/
- sqlc repo: https://github.com/sqlc-dev/sqlc
- chi docs: https://pkg.go.dev/github.com/go-chi/chi/v5
- chi repo: https://github.com/go-chi/chi
- coder/websocket repo: https://github.com/coder/websocket
- coder/websocket godoc: https://pkg.go.dev/github.com/coder/websocket
- Coder stewardship blog post:
  https://coder.com/blog/websocket
- oapi-codegen repo: https://github.com/oapi-codegen/oapi-codegen
- oapi-codegen godoc:
  https://pkg.go.dev/github.com/oapi-codegen/oapi-codegen/v2
- oapi-codegen-exp (3.1 experimental, NOT for production):
  https://github.com/oapi-codegen/oapi-codegen-exp
- kin-openapi (upstream parser blocking 3.1):
  https://github.com/getkin/kin-openapi
