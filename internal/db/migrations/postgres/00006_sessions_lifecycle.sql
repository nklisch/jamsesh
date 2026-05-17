-- +goose Up
-- +goose StatementBegin
ALTER TABLE sessions ADD COLUMN end_reason TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions ADD COLUMN finalize_locked_by_account_id TEXT REFERENCES accounts(id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE ref_modes (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('sync','isolated')),
    PRIMARY KEY (session_id, ref)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS ref_modes;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions DROP COLUMN IF EXISTS finalize_locked_by_account_id;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions DROP COLUMN IF EXISTS end_reason;
-- +goose StatementEnd
