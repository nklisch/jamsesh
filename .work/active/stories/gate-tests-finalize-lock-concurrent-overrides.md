---
id: gate-tests-finalize-lock-concurrent-overrides
kind: story
stage: drafting
tags: [testing, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Finalize lock state machine — no test for concurrent override race

## Priority
Medium

## Spec reference
Item: `epic-finalize-flow-plan-generation-locks-schema-and-rest`
Acceptance criterion: acquire when another member holds a fresh lock +
`override=true` succeeds, sets `superseded_by_lock_id` on the old row,
points sessions pointer to new caller.

## Gap type
missing test for adversarial / concurrent. The self-FK
insert-before-supersede ordering means two concurrent override callers
could create an inconsistent chain or two active locks.

## Suggested test
```go
// TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins
//   B holds the lock. A and C both call AcquireFinalizeLock with override=true.
//   Race them via channels. Assert: exactly one of {A, C} ends with active lock;
//   no two unrevoked active rows exist for the session;
//   sessions.finalize_locked_by_account_id points at the winner.
```

## Test location (suggested)
`internal/portal/finalize/lock_acquire_test.go`
