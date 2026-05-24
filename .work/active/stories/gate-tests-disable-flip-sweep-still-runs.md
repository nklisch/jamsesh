---
id: gate-tests-disable-flip-sweep-still-runs
kind: story
stage: review
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# SELF_HOST.md disable-flip behavior — sweep runs when create is disabled — has no test

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-reserved-org`

Acceptance criterion: Design decision: "Existing in-flight sessions keep running through their normal idle / hard-cap lifecycles — the destruction sweep continues to fire even when the create endpoint is off."

## Gap type
missing test for valid partition

## Suggested test
```go
func TestWorker_RunsEvenWhenCreateDisabled(t *testing.T) {
    // Worker with Cfg.Enabled=false. Seed an expired session.
    // Run sweep. Assert session destroyed.
}
```
All existing worker tests use default-enabled config.

## Test location (suggested)
`internal/portal/playground/worker_test.go`

## Implementation notes

Verified that `Worker.Run()` and `Worker.sweep()` in `worker.go` make no check
on `Cfg.Enabled` — the flag is only read by `CreatePlaygroundSession` and
`JoinPlaygroundSession` in `handler.go`. The worker correctly runs regardless
of the Enabled setting, matching the SELF_HOST.md specification.

Added `TestWorker_RunsEvenWhenCreateDisabled` to `worker_test.go`. The test
constructs a Worker with `Cfg.Enabled=false`, seeds an expired session (hard
cap 1s, clock advanced 2s past), runs one sweep via `runWorkerSweep`, and
asserts the session is destroyed (ErrNotFound) with tombstone `end_reason:
hard_cap`. Test passes, pinning the contract.

Also fixed a pre-existing test-debt issue: `destruction_test.go` imported the
`sync` package but never used it, causing a build failure that blocked the
test run. Removed the unused import.
