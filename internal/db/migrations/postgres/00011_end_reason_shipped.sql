-- +goose Up
-- Widen archived_sessions.end_reason CHECK constraint to include 'shipped'.
-- +goose StatementBegin
ALTER TABLE archived_sessions DROP CONSTRAINT IF EXISTS archived_sessions_end_reason_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE archived_sessions ADD CONSTRAINT archived_sessions_end_reason_check
    CHECK (end_reason IN ('finalize','abandon','timeout','shipped'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE archived_sessions DROP CONSTRAINT IF EXISTS archived_sessions_end_reason_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE archived_sessions ADD CONSTRAINT archived_sessions_end_reason_check
    CHECK (end_reason IN ('finalize','abandon','timeout'));
-- +goose StatementEnd
