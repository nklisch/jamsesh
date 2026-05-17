---
id: epic-portal-foundation-tokens-token-core-and-middleware
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-foundation-tokens
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Tokens — Core, Middleware, Basic-Auth Helper

## Scope

Build the token subsystem core: types, secure generation, SHA-256
hashing, the Service interface and implementation backed by the
data-layer Store, Bearer middleware for `/api/*`, and the HTTP
Basic-auth validator that the git smart-HTTP handler will call.

After this story, sibling features can validate tokens via either
Bearer or Basic, and the auth-flows feature can call `Service.Issue`
to mint pairs.

## Units delivered

- `internal/portal/tokens/service.go` — Service interface, Pair
  struct, lifetime constants, sentinel errors
- `internal/portal/tokens/token.go` — `generateToken()`,
  `hashToken()`
- `internal/portal/tokens/service_impl.go` — concrete Service
  with Clock injection
- `internal/portal/tokens/middleware.go` — `BearerMiddleware`,
  `AccountFromContext`
- `internal/portal/tokens/basic.go` — `BasicAuthValidator`
- Co-located `_test.go` files for each

## Acceptance Criteria

- [ ] `generateToken()` produces 64-char hex tokens; two
      consecutive calls produce different tokens (entropy test)
- [ ] `hashToken(raw)` is deterministic — same input, same output
- [ ] `Service.Issue` against an in-memory SQLite Store produces
      two rows in `oauth_tokens` (access + refresh) with correct
      `kind`, `issued_at`, `expires_at`
- [ ] `Service.Validate` returns the account for a valid token;
      `ErrInvalidToken` for an unknown one; `ErrExpiredToken` for
      a token past `expires_at`; `ErrRevokedToken` for a revoked
      token
- [ ] `Service.Refresh` revokes the old refresh token and mints a
      new pair; the new pair validates; the old refresh token
      returns `ErrRevokedToken` on re-presentation
- [ ] `Service.Revoke(tok, false)` revokes one token;
      `Service.Revoke(tok, true)` revokes every token for the
      account
- [ ] `BearerMiddleware` rejects missing/invalid/expired tokens
      with the correct PROTOCOL.md error code (401 + envelope);
      passes through valid requests with the account attached
- [ ] `BasicAuthValidator(svc)` returns the account when password
      is a valid token; same sentinels on failure
- [ ] All tests green: `go test ./internal/portal/tokens/...`

## Notes

- The injectable `Clock` is critical for testing expiry. Default
  is `realClock` (`time.Now().UTC()`); tests use a fake clock to
  fast-forward through TTLs.
- `TouchOAuthTokenLastUsed` is fire-and-forget — errors are
  swallowed (logged via slog at debug level if instrumentation
  matters). Validation correctness does not depend on the touch
  succeeding.
- The refresh-token race condition (concurrent uses) is left as
  a documented limitation. v1 doesn't add transactional locking
  around Refresh.
- `BasicAuthValidator` signature takes `(ctx, user, pass)` but
  ignores `user` — git's HTTP Basic uses an arbitrary username.
  The user param is kept for callers that may want to log it.

## Wiring (mount in main.go)

Add to `cmd/portal/main.go`:

```go
tokenSvc := tokens.New(s)  // s is the store from db.Open
handler := router.New(router.Deps{
    TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
    MountAPI: func(r chi.Router) {
        r.Use(tokens.BearerMiddleware(tokenSvc))
        // strict-server handlers mount here, but tokens/refresh and revoke
        // need different routing — see next story
    },
})
```

(The refresh-and-revoke story refines the mount shape; this story
just adds the middleware infrastructure.)
