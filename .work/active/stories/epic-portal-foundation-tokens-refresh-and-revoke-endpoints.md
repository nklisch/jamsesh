---
id: epic-portal-foundation-tokens-refresh-and-revoke-endpoints
kind: story
stage: review
tags: [portal, security]
parent: epic-portal-foundation-tokens
depends_on: [epic-portal-foundation-tokens-token-core-and-middleware]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Tokens — Refresh and Revoke REST Endpoints

## Scope

Add the first two real REST endpoints to `docs/openapi.yaml`,
regenerate the typed clients, implement the strict-server handler
methods, and mount them via `router.Deps.MountAPI`.

After this story, `POST /api/auth/refresh` and `POST /api/auth/revoke`
work end-to-end.

## Units delivered

- `docs/openapi.yaml` (edit) — add `/api/auth/refresh` and
  `/api/auth/revoke` paths, `TokenPair` schema, request bodies
- Regenerated `internal/api/openapi/server.gen.go` (committed)
- Regenerated `frontend/src/lib/api/types.gen.ts` (committed)
- `internal/portal/tokens/handlers.go` — implements the generated
  `StrictServerInterface` methods for these two paths
- `cmd/portal/main.go` (edit) — wire the tokens.Handler into
  `router.Deps.MountAPI`, mount `/api/auth/refresh` WITHOUT Bearer
  middleware (public) and `/api/auth/revoke` WITH Bearer middleware
- Handler tests via `httptest`

## Acceptance Criteria

- [ ] `docs/openapi.yaml` lints cleanly as 3.0.3
- [ ] `make generate && git diff --exit-code` is green after the
      regen
- [ ] `POST /api/auth/refresh` with a valid refresh token returns
      200 + `TokenPair`; the old refresh token is now revoked
- [ ] `POST /api/auth/refresh` with an invalid / expired / already-
      revoked token returns 401 with the standard envelope
- [ ] `POST /api/auth/revoke` (Bearer) revokes the supplied token
      and returns 204
- [ ] `POST /api/auth/revoke` with `revoke_all: true` revokes
      every token for the authenticated account
- [ ] `POST /api/auth/revoke` without Bearer returns 401
- [ ] Handler tests green via `httptest.NewServer` exercising the
      full request → handler → store path against in-memory SQLite

## Notes

- Mounting `/api/auth/refresh` PUBLIC is intentional — the
  refresh token IS the credential. The Bearer middleware applies
  ONLY to authenticated routes. The chi router shape uses two
  separate `r.Route` blocks or selective `r.With(BearerMW)` calls
  per route group; choose what reads cleanest.
- The `revoke_all: true` path is an explicit logout-everywhere
  affordance. The auth-flows feature will eventually expose a
  "log out all sessions" UI surface that calls this.
- Generated method names depend on `operationId`s in the spec.
  Use `refreshToken` and `revokeToken` as documented.
- Once these endpoints land, the empty `paths` situation is
  resolved — the generated `EventEnvelope` discriminated union
  for the WebSocket primitive will still be empty (no events
  yet), but the REST surface gains real types.

## Implementation notes

### Wiring choice: per-route group mounting

`HandlerFromMux` registers routes at their full absolute paths
(`/api/auth/refresh`, `/api/auth/revoke`). Since `MountAPI` receives a
chi sub-router already scoped at `/api`, using `HandlerFromMux` would
produce double-prefixed paths (`/api/api/auth/*`). Instead, the strict
handler's individual methods (`strictAPI.RefreshToken`,
`strictAPI.RevokeToken`) are registered directly on two `r.Group` blocks:

```go
// Public (no middleware)
r.Group(func(r chi.Router) {
    r.Post("/auth/refresh", strictAPI.RefreshToken)
})
// Bearer-authenticated
r.Group(func(r chi.Router) {
    r.Use(tokens.BearerMiddleware(tokenSvc))
    r.Post("/auth/revoke", strictAPI.RevokeToken)
})
```

This gives clean per-route middleware isolation without any adapter shims.

### OpenAPI spec note

The spec uses `openapi: 3.0.3` (oapi-codegen mainline 3.1 blocker).
`TokenPair` schema added to `components.schemas`. Both paths reference
`#/components/responses/Unauthorized` for 401.

### Test coverage

`internal/portal/tokens/handlers_test.go` exercises the full
request → handler → store round-trip via an in-memory SQLite store:
- Refresh: valid token → 200 + TokenPair fields present
- Refresh: invalid/access/expired/reused token → 401 with error field
- Revoke: with Bearer → 204
- Revoke: without Bearer → 401
- Revoke: revoke_all → 204 + both tokens invalid afterward
- Revoke: revoked bearer rejected on next request
