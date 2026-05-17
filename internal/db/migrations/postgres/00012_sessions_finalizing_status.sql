-- +goose Up
-- Widen sessions.status CHECK constraint to include 'finalizing'.
-- The finalize-flow lock handler sets status = 'finalizing' when a lock
-- is acquired, but migration 00010_finalize_locks.sql omitted the
-- corresponding constraint update. This migration repairs the gap.
-- +goose StatementBegin
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_status_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
    CHECK (status IN ('active','finalizing','ended','archived'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_status_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
    CHECK (status IN ('active','ended','archived'));
-- +goose StatementEnd
