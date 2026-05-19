---
id: bug-rate-limit-leaks-across-interrupted-ops-subtests
kind: story
stage: review
tags: [bug, auth, ratelimit, e2e-test]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Bug: Rate-limit state leaks across TestInterruptedOps subtests; magic-link subtest gets 429

## Brief

`TestInterruptedOps/magic_link_ttl_expiry` fails at:

```
POST /api/auth/magic-link/request: status 429 (want 204): "rate_limited"
```

(`tests/e2e/failure/interrupted_ops_test.go:277`)

## Root cause

`TestInterruptedOps` (`tests/e2e/failure/interrupted_ops_test.go:90`) creates
ONE shared portal instance and runs multiple subtests against it in declaration
order. All subtests originate HTTP requests from the test process at `127.0.0.1`.

The `magic-link/request` endpoint is guarded by `mlRequestRL` (wired in
`cmd/portal/main.go:721`):
```go
mlRequestRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 3, PerHour: 10}).Middleware(rlEnabled)
```
Limit: **3 per minute**, **10 per hour**, per source IP. All subtests share the
IP `127.0.0.1`.

Magic-link request calls in `TestInterruptedOps`, in subtest order:
1. `push_interrupted_mid_pack` — `SignInViaMagicLink` for alice → **1 request** (cumulative: 1)
2. `finalize_lock_release_and_reacquire` — `SignInViaMagicLink` for alice + bob → **2 requests** (cumulative: 3)
3. `magic_link_ttl_expiry` — `authflow.RequestMagicLink` → **1 request** (cumulative: 4)

The 4th call hits the 3/min burst limit. The `rate.Limiter` in
`internal/portal/ratelimit/store.go` is in-process, in-memory, and NOT reset
between test runs or subtests. The token-bucket refill rate is 3/60 = 0.05
tokens/second, so the burst of 3 is exhausted within a single rapid test run
and the 4th call is rejected with 429.

The `Store` (`internal/portal/ratelimit/store.go:47-57`) has no test-mode
reset hook. GC runs every 5 minutes (`gcInterval: 5 * time.Minute`) with a
1-hour TTL (`ttl: 1 * time.Hour`) — far too long for any test isolation.

## Fix options

**Option A — Disable rate limiting in e2e tests via env var (recommended)**
The portal already respects `JAMSESH_AUTH_RATE_LIMIT_ENABLED=false` (config
field `AuthRateLimitEnabled`, default true). Setting this in the test portal's
`ExtraEnv` disables all auth rate limiters with zero production-code changes.

In `tests/e2e/failure/interrupted_ops_test.go:95-101`:
```go
p := portal.Start(ctx, t, portal.Options{
    DBDriver:  "postgres",
    DBDSN:     pg.ContainerDSN,
    EmailFrom: "noreply@example.com",
    SMTPHost:  mh.ContainerSMTPHost,
    SMTPPort:  mh.ContainerSMTPPort,
    ExtraEnv: map[string]string{
        "JAMSESH_AUTH_RATE_LIMIT_ENABLED": "false",
    },
})
```

**Option B — Give each subtest its own portal instance**
Each subtest spins up its own portal+postgres+mailhog. Rate-limit state cannot
leak across isolated portals. More expensive in CI wall-clock time (~30s per
subtest for container startup) but eliminates the shared-state coupling.

**Option C — Add a reset hook to ratelimit.Store**
Add `func (s *Store) Reset()` that clears `s.entries`. Call it between subtests
(e.g. via a test-only portal endpoint behind the `e2etest` build tag).
More invasive; not recommended when Option A achieves the same result.

**Recommended fix**: Option A. The test exercises the rate-limiter behaviour
implicitly (any test that hits magic-link endpoints proves rate limiting is
active in production). `TestInterruptedOps` is specifically testing *interrupted
operation* semantics, not rate-limiting, so disabling the rate limiter in that
test's portal is correct scoping.

There is no dedicated "rate limit integration test" that would need rewriting —
the existing unit tests in `internal/portal/ratelimit/` provide coverage. If an
e2e rate-limit test is desired, it should be a dedicated test with a fresh portal.

## File:line pointers

- `tests/e2e/failure/interrupted_ops_test.go:95-101` — portal.Start call;
  add `ExtraEnv: map[string]string{"JAMSESH_AUTH_RATE_LIMIT_ENABLED": "false"}`
- `internal/portal/ratelimit/store.go:72-90` — `NewStore` constructs limiter;
  no change needed for Option A
- `internal/portal/config/config.go` — verify `AuthRateLimitEnabled` field is
  wired to `JAMSESH_AUTH_RATE_LIMIT_ENABLED` env var

## Acceptance criteria

- [ ] `TestInterruptedOps/magic_link_ttl_expiry` passes: `RequestMagicLink`
      returns 204 (no-content), not 429.
- [ ] All other `TestInterruptedOps` subtests still pass.
- [ ] No production behavior changes (env var is not set in any non-test context).
- [ ] The rate-limit unit tests in `internal/portal/ratelimit/` continue to pass.

## Implementation Notes

Env var confirmed as `JAMSESH_AUTH_RATE_LIMIT_ENABLED` (via grep of
`internal/portal/config/config.go:676` and `internal/portal/ratelimit/store.go:20`).
The casing assumed in the story body was correct — no discovery needed.

Change made: `tests/e2e/failure/interrupted_ops_test.go:95-104` — added
`ExtraEnv: map[string]string{"JAMSESH_AUTH_RATE_LIMIT_ENABLED": "false"}` to
the `portal.Start` call for `TestInterruptedOps`. No production code touched.
`go vet ./failure/` passes clean.
