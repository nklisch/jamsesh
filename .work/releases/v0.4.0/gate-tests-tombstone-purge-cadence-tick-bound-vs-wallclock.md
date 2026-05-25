---
id: gate-tests-tombstone-purge-cadence-tick-bound-vs-wallclock
kind: story
stage: done
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `PurgeExpiredTombstones` cadence (`purgeEvery = 60` ticks) under non-default sweep interval untested

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Story 2 AC: "Tombstones older than TombstoneTTL are purged by a separate sub-routine." Review nit: "`Worker.purgeEvery` is a const 60 tick count rather than a wall-clock interval. At default 60s sweep this means purge ~1/hour; any other interval drifts proportionally."

## Gap type
missing test for adversarial-spec-silent

## Suggested test
Add an explicit test documenting that purge cadence is tick-bound (not
wall-clock-bound), so a future refactor can't silently change it.

## Test location (suggested)
`internal/portal/playground/worker_test.go`

## Implementation notes

Added `TestWorker_PurgeCadence_IsTickBound_Not_WallClockBound` in
`internal/portal/playground/worker_test.go` with two sub-tests:

- `no purge before 60 ticks regardless of elapsed time` — runs the worker
  for ~30 ticks (30ms at 1ms/tick) using a `purgeCountStore` wrapper and
  asserts `PurgeExpiredTombstones` is never called. Documents that the
  cadence gate is tick-count-based, not elapsed-time-based.
- `purge fires at least once after >=60 ticks` — runs the worker for 200ms
  (~200 ticks) and asserts the purge method was called at least once.

The `purgeCountStore` wrapper delegates all store calls to the real store and
increments an `int` counter on each `PurgeExpiredTombstones` call. No mocking
framework required.

## Review notes

Approve. Wrapper-store count + dual-sided assertions (no-fire under 60 ticks,
≥1-fire above) pin the tick-bound semantics. Real worker.Run() exercises the
production ticker loop. Timing uses generous 30ms/200ms margins relative to
the 60-tick threshold at 1ms/tick. Tests pass.
