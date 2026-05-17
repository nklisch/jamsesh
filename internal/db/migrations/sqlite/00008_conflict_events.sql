-- +goose Up
CREATE TABLE conflict_events (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    source_commit TEXT NOT NULL,
    draft_tip TEXT NOT NULL,
    ancestor TEXT NOT NULL,
    conflicts TEXT NOT NULL,     -- JSON
    addressed_to TEXT NOT NULL,  -- JSON
    status TEXT NOT NULL CHECK (status IN ('open','resolved')),
    resolving_commit_sha TEXT,
    created_at DATETIME NOT NULL,
    resolved_at DATETIME
);
CREATE INDEX conflict_events_session_status_idx ON conflict_events(session_id, status);

-- +goose Down
DROP INDEX IF EXISTS conflict_events_session_status_idx;
DROP TABLE IF EXISTS conflict_events;
