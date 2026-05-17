-- +goose Up
CREATE TABLE finalize_locks (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    acquired_by_account_id TEXT NOT NULL REFERENCES accounts(id),
    acquired_at DATETIME NOT NULL,
    last_activity_at DATETIME NOT NULL,
    selected_commit_shas TEXT NOT NULL DEFAULT '[]',
    target_branch TEXT NOT NULL DEFAULT '',
    base_sha TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT 'squash'
        CHECK (mode IN ('squash','preserve')),
    commit_message TEXT,
    superseded_by_lock_id TEXT REFERENCES finalize_locks(id),
    released_at DATETIME
);
CREATE INDEX finalize_locks_session_idx ON finalize_locks(session_id);
CREATE INDEX finalize_locks_active_idx ON finalize_locks(session_id)
    WHERE released_at IS NULL AND superseded_by_lock_id IS NULL;

-- +goose Down
DROP INDEX IF EXISTS finalize_locks_active_idx;
DROP INDEX IF EXISTS finalize_locks_session_idx;
DROP TABLE IF EXISTS finalize_locks;
