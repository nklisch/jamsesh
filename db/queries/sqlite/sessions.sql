-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at,
                      last_substantive_activity_at, hard_cap_at, idle_timeout_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = ? AND id = ?;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = ?
ORDER BY created_at DESC;

-- name: UpdateSessionStatus :exec
UPDATE sessions
SET status = ?
WHERE org_id = ? AND id = ?;

-- name: SetSessionBaseSHA :exec
UPDATE sessions
SET base_sha = ?
WHERE org_id = ? AND id = ?;

-- name: UpdateSessionGoalScopeMode :exec
UPDATE sessions
SET goal = ?, writable_scope = ?, default_mode = ?
WHERE org_id = ? AND id = ?;

-- name: SetSessionEndReason :exec
UPDATE sessions
SET end_reason = ?, ended_at = ?
WHERE org_id = ? AND id = ?;

-- name: SetFinalizeLock :exec
UPDATE sessions
SET finalize_locked_by_account_id = ?
WHERE org_id = ? AND id = ?;

-- name: ClearFinalizeLock :exec
UPDATE sessions
SET finalize_locked_by_account_id = NULL
WHERE org_id = ? AND id = ?;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE org_id = ? AND id = ?;

-- name: ListSessionsForOrgWithCursor :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason,
       finalize_locked_by_account_id, last_substantive_activity_at, hard_cap_at, idle_timeout_at
FROM sessions
WHERE org_id = ? AND created_at < ?
ORDER BY created_at DESC
LIMIT ?;

-- name: NicknameTakenInSession :one
SELECT COUNT(*) > 0 AS taken
FROM session_members sm
JOIN accounts a ON a.id = sm.account_id
WHERE sm.org_id = ? AND sm.session_id = ? AND a.display_name = ?;

-- name: CountSessionMembers :one
SELECT COUNT(*) AS member_count
FROM session_members
WHERE org_id = ? AND session_id = ?;

-- name: GetTombstone :one
SELECT session_id, org_id, members_count, commits_count, auto_merges_count,
       duration_seconds, end_reason, ended_at, expires_at
FROM tombstones
WHERE session_id = ?;

-- name: RecordTombstone :exec
INSERT INTO tombstones (session_id, org_id, members_count, commits_count, auto_merges_count,
                        duration_seconds, end_reason, ended_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id) DO NOTHING;

-- name: ResetSessionIdleTimer :exec
UPDATE sessions
SET last_substantive_activity_at = ?, idle_timeout_at = ?
WHERE org_id = ? AND id = ?;
