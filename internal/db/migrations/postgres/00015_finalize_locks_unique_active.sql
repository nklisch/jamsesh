-- +goose Up
-- Enforce the invariant that at most one active (non-superseded, non-released)
-- finalize lock may exist per session at any time. The partial unique index
-- covers only rows where both superseded_by_lock_id IS NULL and
-- released_at IS NULL — the set that represents "currently active" locks.
-- This causes a concurrent INSERT in the override path to receive a
-- unique-constraint violation (SQLSTATE 23505) rather than silently
-- creating two active rows.
CREATE UNIQUE INDEX finalize_locks_one_active_per_session_idx
    ON finalize_locks (session_id)
    WHERE superseded_by_lock_id IS NULL AND released_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS finalize_locks_one_active_per_session_idx;
