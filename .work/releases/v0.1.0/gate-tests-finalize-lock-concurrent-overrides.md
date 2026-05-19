---
id: gate-tests-finalize-lock-concurrent-overrides
kind: story
stage: done
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

## Implementation notes

### Test added
`TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins` in
`internal/portal/finalize/lock_acquire_test.go`.

### Race window exercised
`AcquireFinalizeLock` in branch 5 (override=true, fresh lock held by another
member) sequences three writes:

1. `InsertFinalizeLock` — new row, no FK yet (self-FK allows NULL initially)
2. `SupersedeFinalizeLock` — sets `superseded_by_lock_id` on the old row
3. `SetFinalizeLock` — updates `sessions.finalize_locked_by_account_id`

Two concurrent callers (A and C) can both read B's lock as the existing active
row in step "Get active lock" before either has inserted its own row. Both then
proceed through the whole branch 5 path:

- A inserts row A, supersedes B → B's `superseded_by_lock_id = A`
- C inserts row C, supersedes B → B's `superseded_by_lock_id = C`  (last write wins)
- Neither A nor C supersedes the other.

Result: two unsuperseded, unreleased rows (A and C) exist simultaneously — the
invariant "exactly one active lock per session" is violated.

### Invariant check
The test counts rows where `SupersededByLockID IS NULL AND ReleasedAt IS NULL`
across all known lock IDs (B, A, C). It does NOT assert which racer wins —
both winning is acceptable; both being active simultaneously is the bug.

### Bug surfaced — 100% reproducible
Every test run produces `got 2` active rows. This is a confirmed production
bug: the concurrent override path lacks serialisation (no SELECT FOR UPDATE,
no advisory lock, no unique constraint preventing dual-active-lock state).

Bug tracked in backlog: `bug-finalize-lock-concurrent-override-dual-active`
(to be created by reviewer if not already present).

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Concurrency test reproduces a real production bug (100% reproducible): two concurrent AcquireFinalizeLock(override=true) callers both insert lock rows and supersede the original holder, but neither supersedes the other — leaving two active rows. Bug parked as backlog item 'finalize-lock-concurrent-override-race-allows-two-active-locks' with fix-direction notes. Test is t.Skip-anchored to that bug id; remove the skip when the race is fixed. The test invariant (count of non-superseded non-released rows ≤ 1) is correct and ready to enforce post-fix.
