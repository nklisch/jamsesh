-- +goose Up
-- resume_tokens stores short-lived, single-use tokens that allow a CLI client
-- to resume (re-authenticate into) an existing session via the browser portal.
-- ConsumeResumeToken is a winner-returning atomic UPDATE … RETURNING that
-- validates not-used AND not-expired in a single statement, preventing
-- concurrent double-issue at the SQL level.

-- +goose StatementBegin
CREATE TABLE resume_tokens (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    session_id  TEXT NOT NULL,
    org_id      TEXT NOT NULL,
    account_id  TEXT NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS resume_tokens;
-- +goose StatementEnd
