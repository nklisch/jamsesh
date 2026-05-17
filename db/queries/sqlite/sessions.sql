-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
FROM sessions
WHERE org_id = ? AND id = ?;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
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
