-- +goose Up
-- Add playground-specific columns to sessions.
--
-- last_substantive_activity_at: NOT NULL; updated by push/comment/finalize-attempt.
--   Defaults to created_at for existing rows so the column is immediately
--   usable by the destruction-sweep worker without a full table backfill.
--
-- hard_cap_at: nullable; set only for playground sessions. The destruction
--   worker ends the session when now() > hard_cap_at.
--
-- idle_timeout_at: nullable; set only for playground sessions. Stores the
--   explicit deadline derived from (last_substantive_activity_at + IdleTimeout).
--   Stored explicitly for cheap sweep queries — avoids recomputing each tick.
--   Reset by ResetSessionIdleTimer on every substantive event.

-- SQLite cannot ADD NOT NULL columns with a computed default from another
-- column in a single ALTER statement; we add it as nullable first, back-fill
-- from created_at, then use the NOT NULL column in new inserts via the schema.
-- Existing rows will have a valid timestamp after the UPDATE below.
ALTER TABLE sessions
  ADD COLUMN last_substantive_activity_at DATETIME;

UPDATE sessions
  SET last_substantive_activity_at = created_at
  WHERE last_substantive_activity_at IS NULL;

ALTER TABLE sessions
  ADD COLUMN hard_cap_at DATETIME;

ALTER TABLE sessions
  ADD COLUMN idle_timeout_at DATETIME;

-- tombstones: records of destroyed playground sessions. The session row is
-- gone by the time a tombstone exists, so there is no FK to sessions.
-- expires_at drives a TTL cleanup sweep (default 30 days after destruction).
CREATE TABLE tombstones (
    session_id         TEXT PRIMARY KEY,
    org_id             TEXT NOT NULL,
    members_count      INTEGER NOT NULL,
    commits_count      INTEGER NOT NULL,
    auto_merges_count  INTEGER NOT NULL,
    duration_seconds   INTEGER NOT NULL,
    end_reason         TEXT NOT NULL,
    ended_at           DATETIME NOT NULL,
    expires_at         DATETIME NOT NULL
);
CREATE INDEX tombstones_expires_idx ON tombstones(expires_at);

-- +goose Down
DROP INDEX IF EXISTS tombstones_expires_idx;
DROP TABLE IF EXISTS tombstones;

-- SQLite cannot DROP COLUMN before 3.35; rebuild sessions without the new columns.
CREATE TABLE sessions_old (
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

INSERT INTO sessions_old
  SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha,
         status, created_at, ended_at, end_reason, finalize_locked_by_account_id
  FROM sessions;

DROP TABLE sessions;
ALTER TABLE sessions_old RENAME TO sessions;
CREATE INDEX sessions_org_idx ON sessions(org_id);
