-- name: CreateResumeToken :one
INSERT INTO resume_tokens (id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetResumeTokenByHash :one
SELECT id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at
FROM resume_tokens
WHERE token_hash = ?;

-- name: ConsumeResumeToken :one
UPDATE resume_tokens
SET used_at = ?
WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
RETURNING *;
