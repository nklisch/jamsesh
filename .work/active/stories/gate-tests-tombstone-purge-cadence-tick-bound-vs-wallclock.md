---
id: gate-tests-tombstone-purge-cadence-tick-bound-vs-wallclock
kind: story
stage: implementing
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
