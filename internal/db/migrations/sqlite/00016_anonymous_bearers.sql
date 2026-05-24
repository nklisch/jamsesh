-- +goose Up
ALTER TABLE accounts
  ADD COLUMN is_anonymous INTEGER NOT NULL DEFAULT 0;

-- Rebuild oauth_tokens to update CHECK constraint and add session_id FK.
-- SQLite can't ALTER a CHECK constraint or add a FK without rebuilding.
CREATE TABLE oauth_tokens_new (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh','anonymous_session_bearer')),
    session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
    issued_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,
    revoked_at DATETIME
);

INSERT INTO oauth_tokens_new (id, account_id, token_hash, kind, session_id,
                              issued_at, expires_at, last_used_at, revoked_at)
SELECT id, account_id, token_hash, kind, NULL,
       issued_at, expires_at, last_used_at, revoked_at
  FROM oauth_tokens;

DROP TABLE oauth_tokens;
ALTER TABLE oauth_tokens_new RENAME TO oauth_tokens;

CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);
CREATE INDEX oauth_tokens_session_idx ON oauth_tokens(session_id)
  WHERE session_id IS NOT NULL;

-- +goose Down
-- Reverse: remove is_anonymous from accounts, rebuild oauth_tokens without
-- session_id and without the anonymous_session_bearer kind value.
-- Note: rows with kind='anonymous_session_bearer' must be deleted first
-- since the original CHECK constraint does not allow them.
DELETE FROM oauth_tokens WHERE kind = 'anonymous_session_bearer';

CREATE TABLE oauth_tokens_old (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh')),
    issued_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    last_used_at DATETIME,
    revoked_at DATETIME
);

INSERT INTO oauth_tokens_old (id, account_id, token_hash, kind,
                              issued_at, expires_at, last_used_at, revoked_at)
SELECT id, account_id, token_hash, kind,
       issued_at, expires_at, last_used_at, revoked_at
  FROM oauth_tokens;

DROP TABLE oauth_tokens;
ALTER TABLE oauth_tokens_old RENAME TO oauth_tokens;

CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);

-- SQLite does not support DROP COLUMN before 3.35.0. Use table rebuild to
-- remove is_anonymous from accounts.
CREATE TABLE accounts_old (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    github_user_id TEXT,
    created_at DATETIME NOT NULL
);

INSERT INTO accounts_old (id, email, display_name, github_user_id, created_at)
SELECT id, email, display_name, github_user_id, created_at
  FROM accounts;

DROP TABLE accounts;
ALTER TABLE accounts_old RENAME TO accounts;
