---
id: idea-playground-join-handler-ttl-inner-branch-coverage
kind: story
stage: done
tags: [portal, playground, testing]
parent: feature-playground-hardening
depends_on: [gate-security-githttp-receivepack-wallclock-not-injected]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

# Cover the ttl<=0 inner branch in JoinPlaygroundSession

## Origin

Surfaced during review of
`story-playground-server-hardening-handler-test-coverage` (review verdict:
approve with comments).

## Problem

The original story acceptance + parent feature Unit 3 design called for a
stepping-clock test that exercises the inner ttl-check branch in
`internal/portal/playground/handler.go:260-265`:

```go
if ttl <= 0 {
    return openapi.JoinPlaygroundSession410JSONResponse(openapi.ErrorEnvelope{
        Error:   "playground.session_ended",
        Message: "this session has ended",
    }), nil
}
```

This branch fires when `HardCapAt` is just slightly in the future at the
outer check (line 219) but the clock's second read (line 258 via
`time.Until`) yields `ttl <= 0` — a real race where a session expires
mid-handler.

Implementation substituted `TestJoinPlaygroundSession_StatusNotActive_Returns410`
(covers the `Status != "active"` branch at line 227-232) for this test. The
substitution covers a *different* 410 path; the inner ttl<=0 branch remains
untested.

The `stepClock` type IS already in `handler_test.go:36-45` ready for use —
left there during implementation specifically for this future test. The
remaining work is to write the test that uses it.

## Fix

Add `TestJoinPlaygroundSession_TTLZero_Returns410` that:

1. Uses `stepClock` with `step: 2 * time.Second` (or similar)
2. Pre-creates a session with `HardCapAt = clk.Now().Add(1 * time.Second)`
3. Issues POST /join — first clock read passes outer check; second clock
   read (after bearer-issue) trips the `ttl <= 0` branch
4. Asserts 410 with `error="playground.session_ended"`

Note: `time.Until` uses `time.Now()` not `h.Clock.Now()`. The handler may
need a clock-injection touch-up to actually exercise this branch — verify
how `ttl` is computed. If the inner check uses real wall-clock, the test
needs either a `runtime`-level clock injection or this branch is just
inherently flaky to test (and may warrant a refactor to use `h.Clock`
throughout).

## Acceptance

- A test exercises the `ttl <= 0` branch at handler.go:260-265.
- Implementation may include refactoring handler.go to use `h.Clock` for
  the second time check if `time.Until` is the blocker (this would itself
  be a small product-correctness improvement, since the rest of the
  handler is clock-injected).

## Implementation notes

- Added `stepClock` type (pointer receiver) in `playground/handler_test.go`
  alongside the existing `fixedClock`. Returns `base, base+step, base+2*step, ...`
  on successive `Now()` calls.
- New test `TestJoinPlaygroundSession_TTLZero_Returns410` in `handler_test.go`:
  1. Creates a session with `HardCapAt = T0 + 1s`.
  2. Wires the handler with `&stepClock{base: T0, step: 2*time.Second}`.
  3. Resets `clk.n = 0` after env construction (provisioning consumed a
     Now() call) so the join handler sees the intended sequence: outer
     check at T0 (passes, `!T0.Before(T0+1s)` is false), inner ttl at T0+2s
     (`HardCapAt.Sub(T0+2s) = -1s ≤ 0` fires).
  4. Asserts 410 with `playground.session_ended` envelope.
- Also added a `Logout` panic-stub to `playgroundOnlyStrict` (and to all
  other test shims under `internal/portal/`) for the new operation
  introduced by `feature-auth-signout-backend-revoke-backend` — required
  for the strict-server interface check to compile.

Verified: `go test ./internal/portal/playground/... -count 1 -run TTLZero_Returns410` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: `stepClock` correctly simulates the clock-tick race between outer and inner reads. Test runs against both dialects via `stores(t)` matrix. The `clk.n = 0` reset after `newTestEnvWithClock` is a necessary harness detail (provisioning consumed one Now() call) — captured in a comment. Logout panic-stub propagation across 9 test files is the right cross-cutting consequence of the new strict-server operation.
