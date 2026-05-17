-- +goose Up
-- Widen archived_sessions.end_reason CHECK constraint to include 'shipped'.
-- SQLite does not support ALTER TABLE to modify CHECK constraints, so we
-- table-rebuild (same pattern as 00006_sessions_lifecycle).
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE archived_sessions_new (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal_text TEXT NOT NULL,
    member_account_ids TEXT NOT NULL,
    ended_at DATETIME NOT NULL,
    archived_at DATETIME NOT NULL,
    end_reason TEXT NOT NULL CHECK (end_reason IN ('finalize','abandon','timeout','shipped')),
    final_branch_name TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO archived_sessions_new (session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name)
SELECT session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name
FROM archived_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE archived_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE archived_sessions_new RENAME TO archived_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX archived_sessions_org_idx ON archived_sessions(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE archived_sessions_old (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal_text TEXT NOT NULL,
    member_account_ids TEXT NOT NULL,
    ended_at DATETIME NOT NULL,
    archived_at DATETIME NOT NULL,
    end_reason TEXT NOT NULL CHECK (end_reason IN ('finalize','abandon','timeout')),
    final_branch_name TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO archived_sessions_old (session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name)
SELECT session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name
FROM archived_sessions
WHERE end_reason IN ('finalize','abandon','timeout');
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE archived_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE archived_sessions_old RENAME TO archived_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX archived_sessions_org_idx ON archived_sessions(org_id);
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_keys = ON;
-- +goose StatementEnd
