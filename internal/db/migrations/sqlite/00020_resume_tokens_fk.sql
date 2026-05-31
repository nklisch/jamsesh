-- +goose Up
-- Add FK constraints to resume_tokens so that deleting an account or session
-- automatically cascades to their resume tokens. Without these constraints a
-- deleted account's tokens could linger and (if somehow consumed) reference a
-- missing account row.
--
-- SQLite does not support ADD CONSTRAINT on existing tables, so we recreate
-- the table, copy all rows, drop the old table, and rename. A brief exclusive
-- lock is held for the duration of the transaction.

-- +goose StatementBegin
CREATE TABLE resume_tokens_new (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    org_id      TEXT NOT NULL,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    issued_at   DATETIME NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Copy only rows whose account AND session still exist. Pre-FK, deleting an
-- account/session left orphan resume_tokens behind; copying those into the
-- FK-constrained table would fail with FOREIGN KEY constraint failed. Orphans
-- are stale single-use tokens — dropping them is the correct cleanup.
INSERT INTO resume_tokens_new
SELECT id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at
FROM resume_tokens
WHERE account_id IN (SELECT id FROM accounts)
  AND session_id IN (SELECT id FROM sessions);
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE resume_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resume_tokens_new RENAME TO resume_tokens;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE resume_tokens_old (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    session_id  TEXT NOT NULL,
    org_id      TEXT NOT NULL,
    account_id  TEXT NOT NULL,
    issued_at   DATETIME NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO resume_tokens_old
SELECT id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at
FROM resume_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE resume_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resume_tokens_old RENAME TO resume_tokens;
-- +goose StatementEnd
