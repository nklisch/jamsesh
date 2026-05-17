-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSession :one
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
FROM sessions
WHERE org_id = $1 AND id = $2;

-- name: ListSessionsForOrg :many
SELECT id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at
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

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE org_id = $1 AND id = $2;
