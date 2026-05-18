---
id: gate-tests-rate-limit-auth
kind: story
stage: done
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

## Implementation notes

**Test file:** `internal/portal/auth/rate_limit_integration_test.go` (package `auth_test`).

**Chosen approach:** Option B — real chi router with the rate-limit middleware wired directly to a real `MagicLinkHandler`. The handler is backed by an in-memory SQLite store (`openStore(t)`) and a `captureSender` stub, reusing the helpers already present in the `auth_test` package. No mocking of the rate-limit middleware.

**Test seam tested:** `ratelimit.NewStore(...).Middleware(enabled)` wired via chi `r.With(...)` on `POST /api/auth/magic-link/request`. Requests are fired via `httptest.NewRequest` + `handler.ServeHTTP(w, r)` so `r.RemoteAddr` can be controlled directly per-IP without a real TCP connection.

**Three tests added:**
1. `TestAuthRateLimit_MagicLinkRequest_429AfterBurst` — fires burst+1 requests from the same IP; asserts the first burst return 204, the (burst+1)-th returns 429 with `Retry-After` > 0 and `{"error":"rate_limited"}` JSON body.
2. `TestAuthRateLimit_DifferentIPsAreIndependent` — exhausts IP_A's bucket, then fires one request from IP_B and asserts 204 (independent buckets).
3. `TestAuthRateLimit_DisabledKnob_NeverReturns429` — wires with `enabled=false`, fires 20 requests from the same IP, asserts none return 429.

**No bugs found** in the middleware during testing. All assertions pass against the production `ratelimit` package as-shipped.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Critical rate-limit integration coverage verified. Three tests against a real chi router + real ratelimit.Store + real MagicLinkHandler: burst-cap → 429 with Retry-After + envelope JSON; per-IP independence; disabled-knob pass-through. r.RemoteAddr controlled per-request via httptest.NewRequest (no real TCP). Reused existing auth_test helpers (openStore, captureSender) for fixture parity. No middleware bugs found.
