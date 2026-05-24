-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at,
                      last_substantive_activity_at, hard_cap_at, idle_timeout_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = $1 AND id = $2;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = $1
ORDER BY created_at DESC;

-- name: UpdateSessionStatus :exec
UPDATE sessions
SET status = $1
WHERE org_id = $2 AND id = $3;

-- name: SetSessionBaseSHA :exec
UPDATE sessions
SET base_sha = $1
WHERE org_id = $2 AND id = $3;

-- name: UpdateSessionGoalScopeMode :exec
UPDATE sessions
SET goal = $1, writable_scope = $2, default_mode = $3
WHERE org_id = $4 AND id = $5;

-- name: SetSessionEndReason :exec
UPDATE sessions
SET end_reason = $1, ended_at = $2
WHERE org_id = $3 AND id = $4;

-- name: SetFinalizeLock :exec
UPDATE sessions
SET finalize_locked_by_account_id = $1
WHERE org_id = $2 AND id = $3;

-- name: ClearFinalizeLock :exec
UPDATE sessions
SET finalize_locked_by_account_id = NULL
WHERE org_id = $1 AND id = $2;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE org_id = $1 AND id = $2;

-- name: ListSessionsForOrgWithCursor :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;

-- name: NicknameTakenInSession :one
SELECT COUNT(*) > 0 AS taken
FROM session_members sm
JOIN accounts a ON a.id = sm.account_id
WHERE sm.org_id = $1 AND sm.session_id = $2 AND a.display_name = $3;

-- name: CountSessionMembers :one
SELECT COUNT(*) AS member_count
FROM session_members
WHERE org_id = $1 AND session_id = $2;

-- name: GetTombstone :one
SELECT session_id, org_id, members_count, commits_count, auto_merges_count,
       duration_seconds, end_reason, ended_at, expires_at
FROM tombstones
WHERE session_id = $1;

-- name: RecordTombstone :exec
INSERT INTO tombstones (session_id, org_id, members_count, commits_count, auto_merges_count,
                        duration_seconds, end_reason, ended_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (session_id) DO NOTHING;

-- name: ResetSessionIdleTimer :exec
UPDATE sessions
SET last_substantive_activity_at = $1, idle_timeout_at = $2
WHERE org_id = $3 AND id = $4;

-- name: ListExpiredPlaygroundSessions :many
-- Returns active playground sessions where the hard cap or idle timer has elapsed.
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = $1
  AND status = 'active'
  AND (hard_cap_at IS NOT NULL OR idle_timeout_at IS NOT NULL)
  AND (
        (hard_cap_at IS NOT NULL AND hard_cap_at <= $2)
     OR (idle_timeout_at IS NOT NULL AND idle_timeout_at <= $2)
  );

-- name: PurgeExpiredTombstones :exec
DELETE FROM tombstones
WHERE expires_at <= $1;
