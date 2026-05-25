---
id: gate-tests-hard-cap-idle-timeout-boundary-equality
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

# Exact-boundary tests for `hard_cap_at` and `idle_timeout_at` are missing

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Worker sweeps "sessions where `(now > hard_cap_at OR now > idle_timeout_at)`" — boundary at `now == hard_cap_at` is unspecified but must be deterministic.

## Gap type
missing test for boundary

## Suggested test
```go
func TestWorker_SessionExpiresWhenNowEqualsHardCapAt(t *testing.T) { ... }
func TestWorker_SessionExpiresWhenNowEqualsIdleTimeoutAt(t *testing.T) { ... }
```
Document the chosen behavior (matches SQL strict `>` — boundary excluded) so
future refactors can't silently change it.

## Test location (suggested)
`internal/portal/playground/worker_test.go`

## Implementation notes

Added `TestWorker_SessionExpiresWhenNowEqualsHardCapAt` and
`TestWorker_SessionExpiresWhenNowEqualsIdleTimeoutAt` to
`internal/portal/playground/worker_test.go`.

**Discovered: SQL uses `<=`, not strict `>`.**
The story premise said "SQL uses strict `>` — boundary excluded." The actual
generated SQL in `sqlitestore/sessions.sql.go` uses:
```sql
(hard_cap_at IS NOT NULL AND hard_cap_at <= ?)
OR (idle_timeout_at IS NOT NULL AND idle_timeout_at <= ?)
```
This is `hard_cap_at <= now`, i.e. the boundary `now == hard_cap_at` IS
included — the session IS swept at the exact boundary instant.

**Secondary discovery: `reasonFor` off-by-one at the exact boundary.**
`worker.reasonFor` uses `now.After(hard_cap_at)` which is strict `>`. At
the exact point `now == hard_cap_at`, neither the `hard_cap` nor the `idle`
branch fires, and the method falls through to `"manual"`. So the SQL sweep
picks up the session but assigns reason `"manual"` rather than `"hard_cap"`.
Both tests document this mismatch in comments and assert `"manual"` as the
expected tombstone reason at the boundary.

The tests lock two behaviors:
1. Session IS destroyed when `now == hard_cap_at` / `now == idle_timeout_at`
   (SQL `<=` predicate — boundary included).
2. Tombstone reason at exact boundary is `"manual"` (reasonFor off-by-one).

Any future change that aligns `reasonFor` to use `!now.Before(...)` would
correctly produce `"hard_cap"` / `"idle"` at the boundary; the tests would
need to be updated accordingly, which is the intended forcing function.

## Review notes

Approve. Exemplary "test the truth + park the bug" execution. The story
premise (SQL uses strict `>`) was wrong; the implementation read the real SQL,
discovered `<=`, and additionally surfaced a `reasonFor` off-by-one. Tests
honestly assert `"manual"` (current reality) with inline comments naming the
mismatch. The bug is parked at
`bug-playground-worker-reasonFor-off-by-one-at-exact-boundary` with explicit
"update this test when fixing" guidance. Both tests pass.
