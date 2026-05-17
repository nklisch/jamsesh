-- +goose Up
CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    author_account_id TEXT NOT NULL REFERENCES accounts(id),
    author_kind TEXT NOT NULL CHECK (author_kind IN ('human','agent')),
    anchor_commit_sha TEXT NOT NULL,
    anchor_file_path TEXT,
    anchor_line_start INTEGER,
    anchor_line_end INTEGER,
    body TEXT NOT NULL,
    addressed_to TEXT,
    kind TEXT NOT NULL CHECK (kind IN ('question','suggestion','action-request','fyi')),
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by_account_id TEXT REFERENCES accounts(id),
    resolution_note TEXT
);
CREATE INDEX comments_session_idx ON comments(session_id, created_at);
CREATE INDEX comments_addressed_idx ON comments(addressed_to);

-- +goose Down
DROP INDEX IF EXISTS comments_addressed_idx;
DROP INDEX IF EXISTS comments_session_idx;
DROP TABLE IF EXISTS comments;
