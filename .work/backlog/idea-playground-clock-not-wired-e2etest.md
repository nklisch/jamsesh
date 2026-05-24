---
id: idea-playground-clock-not-wired-e2etest
created: 2026-05-24
tags: [playground, testing, e2e-test]
---

In `cmd/portal/main.go` the playground `Handler` (line ~614) and destruction `Worker` (line ~666) are both wired with `playground.RealClock()` instead of the shared `AdvanceableClock` from `testClockProvider`. The `testClockProvider` in `test_clock_advance.go` has no `playgroundClock()` or `destructionWorkerClock()` accessor, so `POST /test/clock-advance` has zero effect on playground session expiry checks or destruction-worker sweep decisions. This means any e2e test that relies on `p.AdvanceClock` to trigger idle-timeout destruction will silently fail to fire the sweep, because the worker always sees real wall time. Fix: add `playgroundClock() playground.Clock` to `testClockProvider`, wire it into both the `playground.Handler` and `playground.Worker` in `main.go`, and add an integration test confirming that clock-advance causes the next sweep to see expired sessions.
