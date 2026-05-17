-- +goose Up
-- +goose StatementBegin
CREATE TABLE org_invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    inviter_account_id TEXT NOT NULL REFERENCES accounts(id),
    recipient_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    accepted_at DATETIME,
    accepted_by_account_id TEXT REFERENCES accounts(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX org_invites_org_idx ON org_invites(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX org_invites_email_idx ON org_invites(recipient_email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS org_invites;
-- +goose StatementEnd
