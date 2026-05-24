---
id: story-fix-playground-join-handler-unit-test-clock-injection-debt
kind: story
stage: done
tags: [bug, testing, portal, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Repair clock-injection mismatch in two failing playground join unit tests

## Symptom

Two tests in `internal/portal/playground/handler_test.go` fail
deterministically against `main`:

```
$ go test ./internal/portal/playground/ -run 'TestJoinPlaygroundSession_(Success|WithNickname_UsesIt)' -count=1
--- TestJoinPlaygroundSession_WithNickname_UsesIt
    handler_test.go:553: join: want 200, got 410
    handler_test.go:558: want nickname=custom-nick, got ""

--- TestJoinPlaygroundSession_Success
    handler_test.go:537: want non-empty nickname
    handler_test.go:545: want members_count=2, got 0
```

The 410 message indicates `playground.session_ended` (hard-cap or
status check fired). Both tests create a fresh session and then
attempt to join it — neither expects the 410.

## Root cause

These tests were originally parked in
`.work/backlog/bug-playground-join-with-nickname-returns-410-on-fresh-session.md`
(now removed; that bug presumed the production handler was broken).

The two-participant e2e test
`TestPlayground_TwoParticipantJoinMerge` (which DOES join a freshly
created session against the real portal binary + real wall clock) passes
reliably 5/5 runs. So the production handler is correct; the bug is in
the unit suite's clock injection.

Two likely culprits (an implementer should diagnose which applies):

1. **Handler bypass of injected clock**: a code path in
   `internal/portal/playground/handler.go > JoinPlaygroundSession`
   compares against `time.Now()` instead of `h.Clock.Now()`. The
   ended-check (`!h.Clock.Now().UTC().Before(*sess.HardCapAt)` at
   handler.go:219) uses the injected clock, but the TTL math at line
   257-265 (`time.Until(*sess.HardCapAt)`) uses real wall time
   internally. If the unit test's `fixedClock` is set to a time in the
   past relative to wall clock, `time.Until` could return a negative
   value that triggers the 410.

2. **Test setup mismatch**: `newTestEnvWithClock` in `handler_test.go`
   may construct the session with `HardCapAt` derived from the wall
   clock at test-setup time, then the join uses the `fixedClock` to
   read "now" but `time.Until(HardCapAt)` uses real time — the
   asymmetry causes the 410 path to fire if real wall time has
   advanced past the test's intended `HardCapAt`.

## Fix approach

Read the failing tests + the JoinPlaygroundSession handler:

- `internal/portal/playground/handler.go` lines 195-302 (JoinPlaygroundSession)
- `internal/portal/playground/handler_test.go > TestJoinPlaygroundSession_Success` (~line 530)
- `internal/portal/playground/handler_test.go > TestJoinPlaygroundSession_WithNickname_UsesIt` (~line 547)
- `internal/portal/playground/handler_test.go > fixedClock` (~line 30) and `newTestEnvWithClock` (~line 264)

Then:

1. **Find the leaking time.Now() call.** Grep
   `internal/portal/playground/handler.go` for `time.Now`,
   `time.Until`, `time.Since` — any of these that should be using
   `h.Clock.Now()` is the bug. Patch them to use the injected clock.

2. **OR fix the test setup.** If the handler is fully clock-injected
   already, the bug is in the test: the session's `HardCapAt` is
   constructed from a different time basis than the clock the handler
   reads. Align them — both should come from `clk.Now().Add(cfg.HardCap)`
   at test setup.

3. **Re-run the failing tests** until they pass against the
   `fixedClock` setup. Then run the full handler_test.go suite to
   confirm no other tests regressed.

4. **Add a unit-level regression**: pin the clock-injection contract
   so this can't drift again. For example: a test that constructs a
   session with `HardCapAt = clock.Now() + 1*time.Hour` and asserts
   that `JoinPlaygroundSession` returns 200 (not 410) even when the
   real wall clock has not advanced. If this test fails, the handler
   is reading wall time somewhere it shouldn't.

## Why this is a fix, not a redesign

The behavior is correct in production (verified by
`TestPlayground_TwoParticipantJoinMerge`). The unit tests have been
silently broken for the whole v0.4.0 release cycle — they were the
canary the e2e suite was supposed to be, and they didn't fire because
the e2e suite didn't exist yet. Now both layers exist; the e2e suite
caught the issue (no real-time bug), and the unit suite needs to be
repaired to match the production contract.

## Regression test

Either the existing `TestJoinPlaygroundSession_Success` and
`TestJoinPlaygroundSession_WithNickname_UsesIt` should pass after the
fix (preferred — they document the contract), OR new tests should
replace them with the corrected clock-injection setup (if the existing
tests have other drift). Implementer's call.

The most honest signal: after the fix, running
`go test ./internal/portal/playground/ -count=1` returns ALL green
(currently it returns those two as the only failures).

## Scope guardrail

This is a single-stride test-debt fix. Do NOT expand into a broader
"clock injection audit across all handlers" — that's a separate refactor
concern. Just fix what's needed for these two tests to pass against
the existing clock-injection contract.

## Related

- `.work/archive/e2e-audit-playground-two-participant-join-merge-journey.md`
  — the e2e test that confirmed production correctness; the
  Implementation Notes there explicitly cite this finding.
- `internal/portal/playground/clock.go` — the per-package Clock
  interface that the handler should be using.
- `.claude/skills/patterns/per-package-clock-interface.md` — the
  project's standard pattern for clock injection.

## Implementation notes

**Root cause: Hypothesis 1 was correct.** The handler at
`internal/portal/playground/handler.go` line 272 called
`time.Until(*sess.HardCapAt)` instead of using the injected clock.

The timeline:
- `newTestEnv` uses a `fixedClock` frozen at `2026-05-23 12:00:00 UTC`
- `CreatePlaygroundSession` runs under this clock, storing
  `HardCapAt = 2026-05-23 12:00:00 + 24h = 2026-05-24 12:00:00 UTC`
- By the time tests run (real wall time `2026-05-24 16:18 UTC`),
  `time.Until(HardCapAt)` returns a negative duration (~-4h)
- The guard `if ttl <= 0` fires and returns 410

The outer guard at line 225 (`!h.Clock.Now().UTC().Before(*sess.HardCapAt)`)
correctly uses `h.Clock.Now()`, which stays frozen at 2026-05-23, so it
passes. But the TTL computation silently leaked to real wall time.

**The fix** (one line, `handler.go:272`):

```go
// Before:
ttl = time.Until(*sess.HardCapAt)
// After:
ttl = sess.HardCapAt.Sub(h.Clock.Now().UTC())
```

This makes the TTL calculation consistent with the outer hard-cap guard.

**Regression test**: No new test was added. The two existing tests
(`TestJoinPlaygroundSession_Success` and
`TestJoinPlaygroundSession_WithNickname_UsesIt`) already serve as the
regression — they use `fixedClock` set to the past, so if the handler
ever reverts to `time.Until`, they will fail again deterministically
once real wall time advances past the frozen `HardCapAt`.

**Full suite**: `go test ./internal/portal/playground/... -count=1` passes
green after the fix.

## Review (2026-05-24)

**Verdict**: Approve

**Notes**:

Hypothesis 1 confirmed: `time.Until(*sess.HardCapAt)` at handler.go:272
bypassed the injected clock. With the test's `fixedClock` frozen at
`2026-05-23 12:00 UTC` and real wall time at `2026-05-24 16:18 UTC`, the
outer hard-cap guard at line 225 (correctly using `h.Clock.Now()`)
let the session through, but the TTL math at line 272 returned a
negative duration via real-time `time.Until`, hitting the `ttl <= 0`
guard at line 277 → 410.

One-line fix:
```go
// Before:
ttl = time.Until(*sess.HardCapAt)
// After:
ttl = sess.HardCapAt.Sub(h.Clock.Now().UTC())
```

Both failing tests now pass; full `internal/portal/playground/...`
package green (0.554s). No new regression test needed — the existing
two tests serve as the pin (any reversion to `time.Until` will
deterministically fail again as wall time advances past the frozen
`HardCapAt`).

The fix preserves the e2e contract (verified by
`TestPlayground_TwoParticipantJoinMerge` still passing) and is narrow
to the single drift site — no other handlers' clock injection touched.

Advanced `stage: review → done`.
