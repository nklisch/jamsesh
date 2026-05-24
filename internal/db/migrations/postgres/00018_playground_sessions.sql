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

ALTER TABLE sessions
  ADD COLUMN last_substantive_activity_at TIMESTAMPTZ;

UPDATE sessions
  SET last_substantive_activity_at = created_at
  WHERE last_substantive_activity_at IS NULL;

ALTER TABLE sessions
  ADD COLUMN hard_cap_at TIMESTAMPTZ;

ALTER TABLE sessions
  ADD COLUMN idle_timeout_at TIMESTAMPTZ;

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
    ended_at           TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX tombstones_expires_idx ON tombstones(expires_at);

-- +goose Down
DROP INDEX IF EXISTS tombstones_expires_idx;
DROP TABLE IF EXISTS tombstones;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS idle_timeout_at;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS hard_cap_at;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS last_substantive_activity_at;
