---
id: epic-portal-foundation-http-skeleton-router-and-middleware
kind: story
stage: done
tags: [portal]
parent: epic-portal-foundation-http-skeleton
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# HTTP Skeleton — Router and Middleware

## Scope

Stand up the chi router chassis with the canonical middleware stack and
the JSON error envelope from `docs/PROTOCOL.md`. After this story, the
portal binary can serve `/healthz` and route 404/405 through the
envelope; subsequent stories add config-driven server lifecycle and
sibling features mount their handlers against `router.Deps`.

## Units delivered

- **Unit 1**: `internal/portal/httperr/httperr.go` — `Error` type +
  `Write` + canonical constructors
- **Unit 2**: `internal/portal/httperr/middleware.go` — `Recoverer`,
  `NotFoundHandler`, `MethodNotAllowedHandler`
- **Unit 3**: `internal/portal/logging/logging.go` — slog `Setup` and
  `Access` middleware
- **Unit 4**: `internal/portal/router/router.go` — `Deps` struct +
  `New(Deps) http.Handler` builder + `/healthz`

## go.mod additions

This story adds `github.com/go-chi/chi/v5` (v5.2.5+). If the data-layer
schema-and-migrations story has not landed yet, this story also
initializes `go.mod` (`module jamsesh`, `go 1.22`).

## Acceptance Criteria

- [ ] `httperr.Error` JSON serializes to exactly the envelope from
      PROTOCOL.md (field order: `error`, `message`, `details`)
- [ ] Panic in a handler returns 500 with the envelope; the panic and
      stack trace are slog-logged at error level with the request ID
- [ ] Unknown route returns 404 with `error: "route.not_found"`
- [ ] Wrong method returns 405 with `error: "route.method_not_allowed"`
- [ ] `GET /healthz` returns 200 with `{"status":"ok"}`
- [ ] Access log line includes method, path, status, duration_ms,
      request_id
- [ ] `router.New(router.Deps{})` (all hooks nil) is a working
      handler; `/api/*`, `/git/*`, `/mcp`, `/ws/*` 404 through the
      envelope
- [ ] All unit tests green: `go test ./internal/portal/...`

## Notes

- Middleware order matters: RequestID → RealIP (conditional) →
  AccessLog → Recoverer → route groups. Parent feature body explains
  why.
- `chi/middleware.RealIP` is installed only when `Deps.TrustProxyHeaders`
  is true. Direct-listening mode must not trust forwarded headers.
- Keep this story scoped to the chassis. Do NOT add config loading or
  `main.go` — those belong to the `config-tls-and-entry` story.

## Implementation notes

### Landed files

- `internal/portal/httperr/httperr.go` — `Error` struct + `Write` + 5 canonical constructors
- `internal/portal/httperr/httperr_test.go` — JSON shape, field presence, constructors, errors.Is/As
- `internal/portal/httperr/middleware.go` — `Recoverer`, `NotFoundHandler`, `MethodNotAllowedHandler`
- `internal/portal/httperr/middleware_test.go` — panic recovery, passthrough, not-found, method-not-allowed
- `internal/portal/logging/logging.go` — `Setup` (json/text formats) + `Access` middleware with `statusRecorder`
- `internal/portal/logging/logging_test.go` — JSON handler setup, access-log field capture (method, path, status, duration_ms, bytes)
- `internal/portal/router/router.go` — `Deps` struct + `New(Deps) http.Handler` + `/healthz`
- `internal/portal/router/router_test.go` — healthz, nil-hook 404s, mounted-hook reach, panic-in-handler, trust-proxy flag, content-type
- `go.mod` / `go.sum` — added `github.com/go-chi/chi/v5 v5.2.5`

### Deviations from design sketches

None material. Minor style improvements:
- Used explicit `opts` variable for `slog.HandlerOptions` rather than inline struct literal in `logging.Setup`.
- Added `http.StatusOK` named constant in `statusRecorder` default init rather than bare `200` for clarity.
- `Access` comment expanded to mention that `slog.InfoContext` carries chi's request ID automatically when it is present in the context.

### Verification

```
go test ./internal/portal/... — 27/27 PASS (httperr: 11, logging: 5, router: 11)
go vet ./...                  — clean
go build ./...                — clean
```

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Middleware order matches design (RequestID → RealIP-gated → AccessLog → Recoverer → routes). The Deps struct's nilable mount hooks are exactly the right late-binding shape — sibling features plug in independently. 27 tests cover the contract. The statusRecorder pattern is clean.
