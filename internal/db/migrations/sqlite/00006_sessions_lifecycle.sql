-- +goose Up
-- Recreate sessions table to extend status CHECK constraint to include 'finalizing',
-- and add end_reason + finalize_locked_by_account_id columns.
-- SQLite does not support ALTER TABLE to modify CHECK constraints.
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE sessions_new (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','finalizing','ended','archived')),
    created_at DATETIME NOT NULL,
    ended_at DATETIME,
    end_reason TEXT,
    finalize_locked_by_account_id TEXT REFERENCES accounts(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO sessions_new (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
FROM sessions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE sessions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions_new RENAME TO sessions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_org_idx ON sessions(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE ref_modes (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('sync','isolated')),
    PRIMARY KEY (session_id, ref)
);
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS ref_modes;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE sessions_old (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','ended','archived')),
    created_at DATETIME NOT NULL,
    ended_at DATETIME
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO sessions_old (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
FROM sessions
WHERE status IN ('active','ended','archived');
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE sessions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE sessions_old RENAME TO sessions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_org_idx ON sessions(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd
