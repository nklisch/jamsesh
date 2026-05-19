---
id: finalize-lock-concurrent-override-race-allows-two-active-locks
kind: story
stage: done
tags: [bug, security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

### Schema change

Added migration `00015_finalize_locks_unique_active` for both dialects:

- `internal/db/migrations/postgres/00015_finalize_locks_unique_active.sql`
- `internal/db/migrations/sqlite/00015_finalize_locks_unique_active.sql`

Both create:

```sql
CREATE UNIQUE INDEX finalize_locks_one_active_per_session_idx
    ON finalize_locks (session_id)
    WHERE superseded_by_lock_id IS NULL AND released_at IS NULL;
```

Mirror entries added to `db/schema/postgres.sql` and `db/schema/sqlite.sql`.

### Error type chosen

`ErrOverrideRaceLost` — declared in `internal/portal/finalize/lock_acquire.go`.

Contract: when `AcquireFinalizeLock(override=true)` returns a 409 with error
code `finalize.override_race_lost`, the caller's INSERT was rejected by the
unique index (another caller's row is now active). No row was inserted for
the loser. The caller should re-query `GetActiveFinalizeLockForSession` to
discover who holds the lock. Do not retry blindly — the winner already holds
the lock.

### Application-code change summary

**Branch 5 (override) in `lock_acquire.go`** was changed to:

1. Call `ReleaseFinalizeLock(existing.ID, now)` BEFORE inserting the new row.
   This removes the existing lock from the unique index's scope (by setting
   `released_at`), allowing the INSERT to proceed.
2. INSERT the new lock row (unique index now allows it since the previous row
   is released).
3. On `ErrUniqueViolation` from the INSERT (concurrent race loser): return a
   409 `finalize.override_race_lost` response with nil Go error.
4. Call `SupersedeFinalizeLock(existing.ID, newLockID)` AFTER the INSERT
   succeeds, preserving the supersede audit trail on the released row.

**Self-FK note**: The self-FK on `superseded_by_lock_id` blocked the
"supersede before insert" ordering. Releasing first breaks the cycle: once
the old row is released (`released_at IS NOT NULL`), it is removed from the
unique partial index's scope, allowing the INSERT. The old row ends up with
both `released_at` and `superseded_by_lock_id` set, which is semantically
correct — it was released AND superseded.

**`lock_patch.go`**: Reordered `released_at` vs `superseded_by_lock_id`
checks so that "superseded" (more actionable) is returned before "released"
when a lock has both fields set.

**Stale test fixtures fixed**:
- `plan_test.go` `TestGetFinalizePlan_LockSuperseded_409`: updated to
  release the first lock before inserting the second.
- `lock_patch_test.go` `TestPatchFinalizeLock_Superseded_409`: same fix.

### Test re-enablement

`t.Skip` removed from `TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins`
in `internal/portal/finalize/lock_acquire_test.go`. Test now passes.

### Verification

- `go build ./...` — clean
- `go test ./internal/portal/finalize/...` — all pass including
  `TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins`
- `go test ./internal/...` — all pass

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- If `InsertFinalizeLock` fails for a non-unique-violation reason (e.g. transient
  DB error) after the prior `ReleaseFinalizeLock` succeeded, the session is
  briefly lockless rather than retaining the prior holder. This is a minor shift
  in error-recovery semantics — defensible (the override was in flight, so the
  prior holder's authority is already questionable), and recoverable on the
  next acquire. Worth keeping in mind if operators report "lock vanished
  mid-override" incidents.
- The `released_at`-now-also-set-on-override behavior means audit readers need
  to look at both fields together: `(released_at IS NOT NULL AND
  superseded_by_lock_id IS NULL)` = voluntary release; `(both NOT NULL)` =
  override. Documented in the implementation notes; consider noting in
  `docs/ARCHITECTURE.md` finalize section if/when that doc grows a lock-state
  table.

**Notes**: Pushing the invariant into the schema (unique partial index) is the
right call — application-only enforcement would have left the race re-openable
by any future code path. The release-first-then-insert ordering is the
necessary consequence of the self-FK on `superseded_by_lock_id` combined with
the new unique index; the agent's analysis traces it cleanly. Stale test
fixtures (which were modeling now-impossible states) were updated rather than
silenced. Re-enabled
`TestAcquireFinalizeLock_ConcurrentOverrides_OnlyOneWins` without modification
to its assertions.
