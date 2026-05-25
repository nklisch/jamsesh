---
id: gate-tests-tombstone-purge-via-worker-not-store-tautology
kind: story
stage: done
tags: [testing, portal, playground, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `TestWorker_PurgesTombstonesAfterTTL` calls store directly — bypasses worker cadence contract

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Implementation notes: "Tombstone purge cadence: Purge runs every 60th sweep tick."

## Gap type
tautological-rework — asserts SQL works, not that worker invokes it on configured cadence

## Suggested test
```go
func TestWorker_PurgesTombstones_OnPurgeEveryTickInterval(t *testing.T) {
    // Run worker for >60 ticks at very short interval.
    // Seed expired tombstones; advance clock; verify purge fires via worker.Run(),
    // not via direct store call.
}
```

## Test location (suggested)
`internal/portal/playground/worker_test.go`

## Implementation notes

Added `TestWorker_PurgesTombstones_OnPurgeEveryTickInterval` in
`internal/portal/playground/worker_test.go`.

The existing `TestWorker_PurgesTombstonesAfterTTL` calls
`env.s.PurgeExpiredTombstones(ctx, now)` directly — it proves the store
method works but says nothing about whether the worker ever invokes it.
The new test drives the full `worker.Run()` path: seeds an expired tombstone,
runs the worker for 200ms at 1ms/tick (well above the 60-tick threshold),
and asserts the tombstone is gone after the run. This exercises the
`worker → purgeTombstones() → store.PurgeExpiredTombstones()` call chain
rather than the store in isolation.

## Review notes

Approve with Important finding. The new test
`TestWorker_PurgesTombstones_OnPurgeEveryTickInterval` correctly drives the
real `worker.Run()` loop and asserts the tombstone is gone — proving the
worker invokes the purge on cadence. This satisfies the story's core goal of
replacing tautology with a real worker-driven test.

Important: the old tautological `TestWorker_PurgesTombstonesAfterTTL`
(internal/portal/playground/worker_test.go:251-286) was NOT removed. The
new test's comment block explicitly says "replaces the existing
TestWorker_PurgesTombstonesAfterTTL", but they currently coexist. Cleanup
spawned at `review-remove-tautological-purge-test`. Tests pass.

### Spawned items
- `review-remove-tautological-purge-test` (Important)
