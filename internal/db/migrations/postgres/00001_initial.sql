-- +goose Up
-- +goose StatementBegin
CREATE TABLE orgs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    github_user_id TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE org_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (org_id, account_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX org_members_account_idx ON org_members(account_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','ended','archived')),
    created_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_org_idx ON sessions(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE session_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    joined_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (session_id, account_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX session_members_org_idx ON session_members(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX session_members_account_idx ON session_members(account_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE oauth_tokens (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh')),
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE magic_link_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS magic_link_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_tokens;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS session_members;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS org_members;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS orgs;
-- +goose StatementEnd
