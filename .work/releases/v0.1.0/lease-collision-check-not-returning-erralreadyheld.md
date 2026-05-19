---
id: lease-collision-check-not-returning-erralreadyheld
kind: story
stage: done
tags: [portal, lease, bug]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# PostgresManager.Acquire collision check does not return ErrAlreadyHeld

## Gap type
bug. `TestPostgresCollisionDefensiveCheck` surfaces a real product defect.

## What fails
`TestPostgresCollisionDefensiveCheck` sets up a row where
`pod_id = "pod-original"` with `released_at IS NULL`, releases the advisory
lock, then calls `mgr.Acquire` from `"pod-collision"`. The test expects
`ErrAlreadyHeld` but `Acquire` returns `nil` — the collision check in
`PostgresManager.Acquire` is not detecting the stale row correctly.

## Test location
`internal/portal/lease/postgres_test.go` — `TestPostgresCollisionDefensiveCheck`

## Fix direction
Inspect `internal/portal/lease/postgres.go` `Acquire` method: the collision
guard should query the `leases` row after acquiring the advisory lock and
return `ErrAlreadyHeld` when `pod_id != mgr.PodID AND released_at IS NULL`.

## Implementation notes

### Root cause

The collision check existed in the original code but was placed **after** the
`InsertLease` upsert (step 4 → step 5). The upsert uses
`ON CONFLICT (session_id) DO UPDATE SET pod_id = EXCLUDED.pod_id, ...` which
unconditionally overwrites the `pod_id` column with the current manager's pod
ID. By the time the check ran, the row always reflected `m.PodID`, so
`rowPodID != m.PodID` was always `false` and the check was permanently
vacuous — it could never return `ErrAlreadyHeld`.

### Fix

Moved the collision check to **before the upsert** (new step 3, before
fencing-token issuance). The sequence is now:

1. Checkout dedicated conn.
2. `pg_try_advisory_lock` — false → `ErrAlreadyHeld`.
3. **Collision check**: `SELECT pod_id, released_at FROM leases WHERE session_id = $1`.
   - If `pod_id != m.PodID AND released_at IS NULL AND pod_id != ""` →
     log warn, release advisory lock, return `ErrAlreadyHeld`.
   - `sql.ErrNoRows` (no row yet) → skip guard, proceed to upsert.
   - Any other scan error → conservative fail, return wrapped error.
4. Issue fencing token.
5. Upsert leases row.
6. Spawn heartbeat, return handle.

The `rowPodID != ""` guard ensures the no-row case (ErrNoRows leaves
`rowPodID` at its zero value `""`) never triggers the collision path.

### Same-pod reacquire is safe

When the same pod reacquires its own lease (e.g., after a restart before
release propagates), `rowPodID == m.PodID`, so the guard condition
`rowPodID != m.PodID` is `false` and Acquire proceeds normally. The upsert
then refreshes `acquired_at`, `fencing_token`, and `heartbeat_at` as
expected.

### Edge cases verified

- **No row exists**: `sql.ErrNoRows` is explicitly excluded; falls through
  to the upsert path.
- **Row exists, released_at IS NOT NULL**: `rowReleasedAt != nil`, so the
  guard does not fire; different pod may acquire.
- **Row exists, same pod**: `rowPodID == m.PodID`, guard does not fire.
- **Row exists, different pod, released_at IS NULL**: guard fires →
  `ErrAlreadyHeld`.

### Test

`TestPostgresCollisionDefensiveCheck` in `internal/portal/lease/postgres_test.go`
had the `t.Skip` sentinel removed. The test now passes: it logs
`lease: hashtext collision detected` and the Acquire call returns
`ErrAlreadyHeld` as expected. All other Postgres lease tests continue to
pass.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none.

**Notes**: Root-cause is correct and clearly explained — `ON CONFLICT DO UPDATE
SET pod_id = EXCLUDED.pod_id` made the post-upsert guard tautological. Reordering
the guard before the upsert is the right fix. The `rowPodID != ""` clause
properly handles `sql.ErrNoRows` (zero-value path), and same-pod reacquire is
preserved because `rowPodID == m.PodID` short-circuits the guard. Added an
`incAcquires("error")` to the collision-scan error path — small consistency
improvement. The skipped test was unskipped without modifying its assertions.
