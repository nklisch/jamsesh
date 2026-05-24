---
id: idea-playground-join-handler-ttl-inner-branch-coverage
kind: story
stage: backlog
tags: [portal, playground, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
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
