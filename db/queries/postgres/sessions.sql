-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason, finalize_locked_by_account_id
FROM sessions
WHERE org_id = $1 AND id = $2;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason, finalize_locked_by_account_id
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
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, end_reason, finalize_locked_by_account_id
FROM sessions
WHERE org_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;
