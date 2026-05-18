---
id: epic-portal-foundation-tokens-token-core-and-middleware
kind: story
stage: done
tags: [portal, security]
parent: epic-portal-foundation-tokens
depends_on: []
release_binding: v0.1.0
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

## Implementation Notes

### Files landed

- `internal/portal/tokens/service.go` — `Service` interface, `Pair` struct, lifetime constants (`AccessTokenTTL` = 1h, `RefreshTokenTTL` = 30d), sentinel errors
- `internal/portal/tokens/token.go` — `generateToken()` (crypto/rand → 32 bytes → 64-char hex) and `hashToken()` (SHA-256 hex)
- `internal/portal/tokens/service_impl.go` — concrete `*service` with `Clock` injection; `New(store)` and `NewWithClock(store, clock)` constructors
- `internal/portal/tokens/middleware.go` — `BearerMiddleware(svc)` and `AccountFromContext(ctx)`
- `internal/portal/tokens/basic.go` — `BasicAuthValidator(svc)`
- `internal/portal/tokens/token_test.go` — generation + hashing unit tests
- `internal/portal/tokens/service_test.go` — full service lifecycle tests against in-memory SQLite
- `internal/portal/tokens/middleware_test.go` — middleware matrix tests with mock service
- `internal/portal/tokens/basic_test.go` — BasicAuthValidator tests against in-memory SQLite

### Clock injection pattern

`Clock` is a single-method interface (`Now() time.Time`). `realClock{}` wraps `time.Now().UTC()`. Tests use `fakeClock{t time.Time}` with an `advance(d)` method to fast-forward through TTLs without sleeping. `NewWithClock(s, c)` is the test-only constructor; production code calls `New(s)`.

### Error mapping

Sentinel errors (`ErrInvalidToken`, `ErrExpiredToken`, `ErrRevokedToken`) are package-level `errors.New` values. `BearerMiddleware` uses `errors.Is` to map them to `httperr.ErrInvalidToken()` or `httperr.ErrExpiredToken()` — both return 401. `ErrRevokedToken` maps to `ErrInvalidToken` in the envelope (intentional: don't leak revocation reason to clients). Internal errors map to `httperr.ErrInternal(err)` (500).

### Deviations from design sketch

1. **ID generation**: used `github.com/google/uuid` (already in go.mod as indirect) instead of `github.com/oklog/ulid/v2` (not in go.mod). IDs are UUID v4 strings stored in the `id` column.
2. **Store method signatures**: all adapted to params-struct calling convention. Key differences:
   - `CreateOAuthToken` returns `(OAuthToken, error)` — return value discarded on write
   - `TouchOAuthTokenLastUsed` takes `TouchOAuthTokenLastUsedParams{ID, LastUsedAt *time.Time}`
   - `RevokeOAuthToken` takes `RevokeOAuthTokenParams{ID, RevokedAt *time.Time}`
   - `RevokeAllOAuthTokensForAccount` takes `RevokeAllOAuthTokensForAccountParams{AccountID, RevokedAt *time.Time}`
3. **BasicAuthValidator simplified**: the design sketch had redundant `errors.Is` re-checking before returning — simplified to a direct `return svc.Validate(ctx, pass)` since `Validate` already returns the correct sentinels.

### Verification

- `go test ./internal/portal/tokens/...` — 31 tests, all green
- `go build ./...` — clean
- `go vet ./...` — clean

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

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Clean implementation. Service interface + injectable Clock for testing + sentinel error mapping in BearerMiddleware. 31 tests cover the contract. UUID used instead of ULID (former is already in go.mod indirectly); reasonable deviation. ErrRevokedToken mapped to ErrInvalidToken in envelope — intentional to avoid leaking revocation reason to attackers.
