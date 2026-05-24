---
id: idea-playground-worker-clock-not-advanceable
created: 2026-05-24
tags: [testing, playground, e2e-test]
---

In e2etest builds, `cmd/portal/main.go` wires `playground.RealClock()` into both the `playground.Handler` and `playground.Worker` instead of the shared `testclock.AdvanceableClock` used by every other subsystem. As a result, `POST /test/clock-advance` does not advance the destruction worker's notion of `time.Now()`, so e2e tests that call `p.AdvanceClock(...)` to push past the hard-cap cannot trigger the sweep without waiting real wall-clock time. The fix is to add a `playgroundClock()` accessor to `testClockProvider` (in `test_clock_advance.go`) and thread `testClk.playgroundClock()` into both the handler and worker in `main.go`, mirroring the pattern already applied to automerger, sessions, finalize, etc.
