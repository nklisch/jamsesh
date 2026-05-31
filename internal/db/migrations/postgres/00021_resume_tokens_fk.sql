-- +goose Up
-- Add FK constraints to resume_tokens so that deleting an account or session
-- automatically cascades to their resume tokens. Without these constraints a
-- deleted account's tokens could linger and (if somehow consumed) reference a
-- missing account row.

-- +goose StatementBegin
-- Purge orphan tokens first: pre-FK, deleting an account/session left resume
-- tokens behind, and ADD CONSTRAINT validates existing rows. Orphans are stale
-- single-use tokens — deleting them is the correct cleanup.
DELETE FROM resume_tokens
WHERE account_id NOT IN (SELECT id FROM accounts)
   OR session_id NOT IN (SELECT id FROM sessions);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resume_tokens
    ADD CONSTRAINT resume_tokens_session_id_fk
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resume_tokens
    ADD CONSTRAINT resume_tokens_account_id_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE resume_tokens DROP CONSTRAINT IF EXISTS resume_tokens_account_id_fk;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE resume_tokens DROP CONSTRAINT IF EXISTS resume_tokens_session_id_fk;
-- +goose StatementEnd
