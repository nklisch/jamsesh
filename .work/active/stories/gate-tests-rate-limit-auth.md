---
id: gate-tests-rate-limit-auth
kind: story
stage: implementing
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-rate-limit-auth-endpoints]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# No rate-limit test exists for `/auth/*` endpoints

## Priority
Critical

## Spec reference
Item: `gate-security-rate-limit-auth-endpoints`
Acceptance criterion: cap `/auth/magic-link/request` and
`/auth/oauth/start` at single-digit RPM per IP+email pair; return 429
with Retry-After header.

## Gap type
missing test for boundary. `grep -rn 'Throttle\|RateLimit' internal/portal/`
returns no production hits.

## Suggested test
```go
// TestRequestMagicLink_RateLimit_Returns429AfterCap
// Fire N+1 POST /auth/magic-link/request from the same IP within window.
// Assert the (N+1)-th returns 429 with Retry-After header.
// Also: TestExchangeMagicLink_RateLimit_Returns429 — guard brute-force exchange.
```

## Test location (suggested)
`internal/portal/auth/magic_link_test.go` (or new `rate_limit_test.go`)
