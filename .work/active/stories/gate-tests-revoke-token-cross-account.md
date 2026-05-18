---
id: gate-tests-revoke-token-cross-account
kind: story
stage: review
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-revoke-token-bearer-account-check]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `RevokeToken` cross-account revocation has zero test coverage

## Priority
Critical

## Spec reference
Item: `gate-security-revoke-token-bearer-account-check`
Acceptance criterion: caller A authenticated as A but submitting
`{token: <B's leaked token>, revoke_all: true}` must NOT revoke B's
tokens. Existing tests in `internal/portal/tokens/handlers_test.go:336-416`
only use the same account for bearer + body token.

## Gap type
missing test for adversarial-spec-silent

## Suggested test
```go
// TestHandler_RevokeToken_CrossAccount_Forbidden
// Two accounts A and B. Bearer = A's access token. Body = B's refresh token,
// revoke_all=true. Expect 403 (or 204 + B's tokens still valid post-fix).
```

## Test location (suggested)
`internal/portal/tokens/handlers_test.go`

## Implementation notes

Added two handler-level integration tests to `internal/portal/tokens/handlers_test.go`:

- `TestHandler_RevokeToken_CrossAccount_Forbidden` — A's bearer + B's refresh token,
  `revoke_all=false`. Asserts HTTP 403 with `{"error": "auth.forbidden"}` and confirms
  B's access and refresh tokens remain valid via `env.svc.Validate`.

- `TestHandler_RevokeToken_CrossAccount_RevokeAll_Forbidden` — A's bearer + B's refresh
  token, `revoke_all=true`. Same 403/body assertions; confirms B's tokens survive.

Both tests drive a real HTTP request through the `httptest.Server` / chi router /
`openapi.NewStrictHandler` / `tokens.Handler` stack — the same wiring used in
production. The handler correctly reaches `Service.Revoke` → `ErrForbidden` →
`RevokeToken403JSONResponse`; no short-circuit before the service call was observed.

All 44 tests in `./internal/portal/tokens/...` pass.
