-- +goose Up
-- Widen events.seq and event_seq.next from INTEGER (32-bit) to BIGINT (64-bit).
--
-- Rationale: the Go domain model and the SQLite dialect already treat seq as
-- int64; Postgres was the outlier.  A 32-bit counter wraps at ~2.1 billion,
-- which is unrealistic for normal sessions but breaks the isomorphic-surface
-- contract.  This is a non-destructive widening — no existing data is lost and
-- no value will be truncated.
--
-- Note: ALTER COLUMN TYPE on Postgres rewrites the affected rows (table-lock
-- briefly held).  Acceptable at current scale; run during a low-traffic
-- maintenance window on larger installations.
--
-- +goose StatementBegin
ALTER TABLE events ALTER COLUMN seq TYPE BIGINT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE event_seq ALTER COLUMN next TYPE BIGINT;
-- +goose StatementEnd

-- +goose Down
-- Intentionally no destructive narrowing: a BIGINT → INTEGER down-migration
-- would silently truncate any seq value > 2^31-1, corrupting event ordering
-- for sessions that have accumulated more than ~2.1 billion events.  The
-- widening is considered a one-way, forward-only schema change.
--
-- If you must roll back to the previous schema version for other reasons,
-- restore from a backup taken before applying this migration.
SELECT 1; -- no-op placeholder so goose sees a valid Down block
