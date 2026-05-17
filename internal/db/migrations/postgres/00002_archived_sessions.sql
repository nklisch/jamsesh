-- +goose Up
-- +goose StatementBegin
CREATE TABLE archived_sessions (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal_text TEXT NOT NULL,
    member_account_ids TEXT NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL,
    end_reason TEXT NOT NULL CHECK (end_reason IN ('finalize','abandon','timeout')),
    final_branch_name TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX archived_sessions_org_idx ON archived_sessions(org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS archived_sessions;
-- +goose StatementEnd
