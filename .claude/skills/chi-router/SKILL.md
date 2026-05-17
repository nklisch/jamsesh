---
name: chi-router
description: Reference for chi v5 HTTP router. Auto-loads when editing Go files that import github.com/go-chi/chi/v5 or github.com/go-chi/chi/v5/middleware, when constructing the portal's HTTP router skeleton, or when wiring per-subroute middleware. Also triggers on terms — chi.NewRouter, chi.Router, r.Route, r.Group, r.Mount, r.Use, chi.URLParam, middleware.Logger, middleware.Recoverer, middleware.RequestID, middleware.RealIP, middleware.Timeout, BasicAuth, multi-auth shape, http.Handler.
user-invocable: false
---

# chi router reference (jamsesh)

**Pinned version**: v5.2.5 (2026-02-05). Module:
`github.com/go-chi/chi/v5`. Middleware:
`github.com/go-chi/chi/v5/middleware`.

chi routers ARE `http.Handler`; chi middleware is
`func(http.Handler) http.Handler`. Drop-in with stdlib anywhere.

## Why chi here (locked decision)

jamsesh has FOUR auth surfaces on one binary:

| Route group       | Auth mechanism                                      |
|-------------------|-----------------------------------------------------|
| `/api/*`          | Bearer (opaque token, validated by foundation)      |
| `/git/*`          | HTTP Basic (username `token`, password = the token) |
| `/mcp`            | Bearer per-request via MCP SDK `getServer` callback |
| `/ws/sessions/{}` | `Sec-WebSocket-Protocol: jamsesh.bearer.<token>`    |

Per-subroute middleware via `r.Route(...)` keeps each auth contained
to its subtree. Stdlib middleware composition gets verbose for this
shape; chi's `Route` block is one line per scope.

## Canonical router skeleton

```go
package portal

import (
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(deps Deps) http.Handler {
    r := chi.NewRouter()

    // Global, ordered: ID → IP → log → recover → timeout.
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(30 * time.Second))

    // REST API — Bearer everywhere under /api.
    r.Route("/api", func(r chi.Router) {
        r.Use(BearerAuth(deps.Tokens))
        // The strict oapi-codegen handler mounts itself onto this subrouter.
        api.HandlerFromMux(api.NewStrictHandler(deps.APIServer, nil), r)
    })

    // Git smart-HTTP — HTTP Basic, token as password.
    r.Route("/git", func(r chi.Router) {
        r.Use(BasicAuthToken(deps.Tokens))
        r.Handle("/*", deps.GitHandler) // wraps git http-backend
    })

    // MCP — bearer is read by the MCP SDK callback per request.
    r.Mount("/mcp", deps.MCPHandler)

    // WebSocket — auth runs INSIDE the handler at upgrade time.
    r.Get("/ws/sessions/{sessionID}", deps.WSHandler.Serve)

    // Catch-all for embedded SPA assets (Svelte build output).
    r.Handle("/*", deps.SPAHandler)

    return r
}
```

## Route patterns

- `{name}` — named param: `/users/{userID}` → `chi.URLParam(r, "userID")`
- `{name:regex}` — constrained: `/articles/{slug:[a-z-]+}`
- `*` — wildcard (matches `/`): `/files/*`
- `/` (trailing) — chi distinguishes trailing slash; use
  `middleware.StripSlashes` or `middleware.RedirectSlashes` to
  normalize.

## Group vs Route vs Mount

- **`r.Use(mw)`** — appends to the current router's middleware stack.
- **`r.With(mw1, mw2).Get(...)`** — inline middlewares for a single
  handler. Use sparingly.
- **`r.Group(func(r){ ... })`** — same prefix, fresh middleware
  stack. Useful for "these N handlers all need this extra middleware
  but share the parent's path".
- **`r.Route("/prefix", func(r){ ... })`** — new prefix AND fresh
  middleware stack. The hammer for jamsesh's multi-auth shape.
- **`r.Mount("/prefix", handler)`** — mount an external
  `http.Handler` (or another chi.Router) under a prefix. Used for the
  MCP SDK handler.

## URL params

```go
sessionID := chi.URLParam(r, "sessionID")
// or from a derived context:
sessionID := chi.URLParamFromCtx(ctx, "sessionID")

// Get the full route pattern (handy for metrics):
pattern := chi.RouteContext(r.Context()).RoutePattern()
// → "/ws/sessions/{sessionID}"
```

## Middleware composition tips

- Order matters: `RequestID` first so logs have it; `Recoverer` AFTER
  `Logger` so a panic still logs the start line.
- Middlewares declared AFTER `r.Mount(...)` / `r.Handle(...)` do NOT
  apply retroactively. Always declare `r.Use(...)` BEFORE the route
  declarations they should wrap.
- Custom middleware:

  ```go
  func BearerAuth(tokens TokenStore) func(http.Handler) http.Handler {
      return func(next http.Handler) http.Handler {
          return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
              h := r.Header.Get("Authorization")
              tok, ok := strings.CutPrefix(h, "Bearer ")
              if !ok {
                  jsonError(w, http.StatusUnauthorized, "auth.missing_bearer")
                  return
              }
              acct, err := tokens.Validate(r.Context(), tok)
              if err != nil {
                  jsonError(w, http.StatusUnauthorized, "auth.invalid_token")
                  return
              }
              ctx := context.WithValue(r.Context(), accountKey{}, acct)
              next.ServeHTTP(w, r.WithContext(ctx))
          })
      }
  }
  ```

## Standard middlewares used by jamsesh

From `github.com/go-chi/chi/v5/middleware`:

- `RequestID` — injects `X-Request-Id` and `middleware.RequestIDKey`
- `RealIP` — honours `X-Forwarded-For` / `X-Real-IP` (gated by
  trusted-proxy config!)
- `Logger` — request line, status, duration
- `Recoverer` — panic → 500 + stack trace
- `Timeout(d)` — wraps the request `context.Context`
- `Compress(level, "application/json", "text/html", ...)` — gzip
- `StripSlashes` — normalize trailing `/`
- `Heartbeat("/healthz")` — drop-in liveness endpoint

## Pitfalls

- **`middleware.RealIP` without proxy config is a spoof vector**. Only
  enable when behind a trusted proxy. jamsesh's TLS-termination
  config has a `behind_proxy: true` flag; gate RealIP on it.
- **`middleware.Timeout` cancels the context but doesn't kill the
  goroutine** — handlers must honour ctx.Done(). The standard library
  http.Server uses the same model.
- **chi v4 → v5**: module path changed (`github.com/go-chi/chi/v5`),
  some middleware moved. Don't import `github.com/go-chi/chi` (v4).
- **NotFound / MethodNotAllowed** are sub-router-specific. Set them
  on the top router for the global default; setting on a subrouter
  scopes the override.
- **chi.URLParam on a route without that param** returns `""`, no
  error. Validate explicitly.
- **r.Mount with chi.Router children**: works, but the mounted
  router's NotFound handler does NOT delegate to the parent. Set 404
  on the parent only.

## References

- Foundation epic:
  `.work/active/epics/epic-portal-foundation.md` (http-skeleton)
- Research doc: `docs/research/core-go-server-stack.md`
- godoc: https://pkg.go.dev/github.com/go-chi/chi/v5
- middleware godoc:
  https://pkg.go.dev/github.com/go-chi/chi/v5/middleware
- Repo: https://github.com/go-chi/chi
