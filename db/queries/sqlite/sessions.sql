-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason, finalize_locked_by_account_id
FROM sessions
WHERE org_id = ? AND id = ?;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason, finalize_locked_by_account_id
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
