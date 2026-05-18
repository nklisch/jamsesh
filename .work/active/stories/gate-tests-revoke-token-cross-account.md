---
id: gate-tests-revoke-token-cross-account
kind: story
stage: implementing
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
