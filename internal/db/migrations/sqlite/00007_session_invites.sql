-- +goose Up
CREATE TABLE session_invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    inviter_account_id TEXT NOT NULL REFERENCES accounts(id),
    invitee_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    accepted_at DATETIME,
    accepted_by_account_id TEXT REFERENCES accounts(id)
);
CREATE INDEX session_invites_session_idx ON session_invites(session_id);
CREATE INDEX session_invites_email_idx ON session_invites(invitee_email);

-- +goose Down
DROP INDEX IF EXISTS session_invites_email_idx;
DROP INDEX IF EXISTS session_invites_session_idx;
DROP TABLE IF EXISTS session_invites;
