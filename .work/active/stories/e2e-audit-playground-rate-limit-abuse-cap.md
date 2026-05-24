---
id: e2e-audit-playground-rate-limit-abuse-cap
kind: story
stage: drafting
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-failure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Per-IP/hour create rate limit has no failure-mode e2e test — abuse-cap journey only covered in unit isolation

## Severity
High

## Finding type
missing-taxonomy-layer

## Evidence

`internal/portal/playground/ratelimit_test.go` has nine unit tests:
`TestNewCreateRateLimiter_PerHourConversion`, `..._FourthCreateBlocked`,
`..._DifferentIPsIndependent`, `..._ZeroHourDefaultsToOne`,
`..._LargePerHour`, `TestCreateRateLimitMiddleware_Disabled`,
`..._Enabled_BlocksAfterBurst`, `..._DifferentIPsSeparateCounters`,
`..._Returns429WithRetryAfter`. All run in-process via `httptest`.

`tests/e2e/failure/rest_validation_test.go:118` references 429 — but only
as a note about test interference, not as a test of the playground rate
limiter:

```
// subtests and later subtests fail with 429 instead of the validation
```

No test in `tests/e2e/` actually triggers
`POST /api/playground/sessions` from the same simulated source IP
repeatedly to validate the 4th-request rejection in a real network +
real-handler context.

## Why this matters

Rate limiting is THE abuse defense for an anonymous-create endpoint. The
unit tests verify the rate limiter struct behaves correctly when called
directly. They do **not** verify:
- The middleware is actually wired into the real router at the right path.
- The IP extraction (`X-Forwarded-For` vs `RemoteAddr`) works correctly
  through the real chi mux + any reverse-proxy header rewriting.
- The 429 response includes a `Retry-After` header at the wire level (not
  just in the unit-test `ResponseRecorder`).
- The limiter persists state across the real portal's request lifecycle
  (singleton vs per-request struct mismatch is a real failure mode).

A wiring bug where the limiter is constructed but never mounted on the
route would pass every unit test and ship a wide-open endpoint.

## Suggested remedy

Add `tests/e2e/failure/playground_create_rate_limit_test.go`. Configure
the portal with `PlaygroundCreatesPerHourPerIP=3` (or whatever env var
controls it). Fire 4 sequential `POST /api/playground/sessions` requests
from the same client. Assert:
1. Requests 1-3 return 201.
2. Request 4 returns 429.
3. Response 4 has a `Retry-After` header > 0.
4. The error envelope shape matches `rest_validation_test.go` conventions
   (typed `error` field like `playground.rate_limited`).

For per-IP isolation: drive request 4 from a second client connecting to
the same portal — assert it gets 201, demonstrating the limit is per-IP
not global.

## Test sketch

```go
// tests/e2e/failure/playground_create_rate_limit_test.go
func TestPlayground_RateLimit_FourthCreateBlocked(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:                       "postgres",
        DBDSN:                          pg.ContainerDSN,
        PlaygroundEnabled:              true,
        PlaygroundCreatesPerHourPerIP:  3,
    })

    for i := 1; i <= 3; i++ {
        resp := postJSON(t, p.URL+"/api/playground/sessions", "", nil)
        require.Equalf(t, 201, resp.StatusCode, "burst request %d", i)
    }

    resp := postJSON(t, p.URL+"/api/playground/sessions", "", nil)
    require.Equal(t, 429, resp.StatusCode)
    require.NotEmpty(t, resp.Header.Get("Retry-After"))

    body := decodeError(t, resp)
    require.Equal(t, "playground.rate_limited", body.Error)
}
```
