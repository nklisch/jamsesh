-- +goose Up
-- Widen tombstones aggregate counters from INTEGER (32-bit) to BIGINT (64-bit).
--
-- Rationale: the Go domain model (store.Tombstone) and the SQLite dialect
-- already treat members_count, commits_count, auto_merges_count, and
-- duration_seconds as int64.  The Postgres schema was the outlier, carrying
-- int32 columns that required explicit int32(...) casts in the adapter.  This
-- breaks the isomorphic-surface contract and would silently truncate any value
-- > 2^31-1, even though overflow is unlikely at current scale.  This mirrors
-- the widening in 00019_seq_bigint.sql applied to events.seq.
--
-- Note: ALTER COLUMN TYPE on Postgres briefly acquires a table lock to rewrite
-- the affected rows.  Acceptable at current scale; prefer a low-traffic window
-- on larger installations.
--
-- +goose StatementBegin
ALTER TABLE tombstones ALTER COLUMN members_count TYPE BIGINT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE tombstones ALTER COLUMN commits_count TYPE BIGINT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE tombstones ALTER COLUMN auto_merges_count TYPE BIGINT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE tombstones ALTER COLUMN duration_seconds TYPE BIGINT;
-- +goose StatementEnd

-- +goose Down
-- Intentionally no destructive narrowing: a BIGINT → INTEGER down-migration
-- would silently truncate any value > 2^31-1, corrupting tombstone aggregate
-- counts.  The widening is a one-way, forward-only schema change.
--
-- If you must roll back to the previous schema version for other reasons,
-- restore from a backup taken before applying this migration.
SELECT 1; -- no-op placeholder so goose sees a valid Down block
