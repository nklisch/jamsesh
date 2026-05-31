-- name: CreateResumeToken :one
INSERT INTO resume_tokens (id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetResumeTokenByHash :one
SELECT id, token_hash, session_id, org_id, account_id, issued_at, expires_at, used_at
FROM resume_tokens
WHERE token_hash = $1;

-- name: ConsumeResumeToken :one
UPDATE resume_tokens
SET used_at = $1
WHERE token_hash = $2 AND used_at IS NULL AND expires_at > $3
RETURNING *;
