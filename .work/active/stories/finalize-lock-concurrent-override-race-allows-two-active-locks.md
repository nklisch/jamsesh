---
id: finalize-lock-concurrent-override-race-allows-two-active-locks
kind: story
stage: implementing
tags: [bug, security, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Two concurrent `AcquireFinalizeLock(override=true)` callers leave two active locks

## Reproducer

Surfaced by `TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins` in
`internal/portal/finalize/lock_acquire_test.go` (currently skipped pending
this fix).

100% reproducible: when accounts A and C both call
`AcquireFinalizeLock(override=true)` against B's active lock concurrently:

1. Both read B as the existing active lock before either inserts their row.
2. Each inserts their own lock row.
3. Each calls `SupersedeFinalizeLock(B)` — last write to B's
   `superseded_by_lock_id` wins, but neither A nor C supersedes the other.

Result: two rows with `superseded_by_lock_id IS NULL AND released_at IS NULL`
exist simultaneously. The invariant "exactly one active lock per session" is
violated.

## Fix direction

Serialise the override path against concurrent acquires. Reasonable options:

- `SELECT ... FOR UPDATE` on the existing active row inside the same tx that
  inserts the new row and supersedes the old one.
- Advisory lock (Postgres) / `BEGIN IMMEDIATE` (SQLite) on the session id at
  the start of the override branch.
- Unique partial index on `(session_id) WHERE superseded_by_lock_id IS NULL
  AND released_at IS NULL` — pushes the invariant into the schema. Most
  durable; requires the schema to support partial indexes (Postgres yes,
  SQLite yes via partial-index syntax).

The unique-partial-index path is the strongest — the database enforces the
invariant and one of the two concurrent inserts gets a constraint violation
to handle. The other paths leave the invariant in application code.

## Impact

Medium-to-high: in a clustered deployment with two collaborators racing to
override the finalize lock, both can end up holding "the" lock from the
portal's perspective. This breaks downstream assumptions (e.g., curation
view "you hold the lock" UI, finalize plan generation, mark-shipped
authorization).

## Story handoff

When picking this up: re-enable the test by removing the `t.Skip` referencing
this story id in `lock_acquire_test.go`. The test already encodes the
correct invariant check (count of non-superseded non-released rows ≤ 1).
