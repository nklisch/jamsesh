-- name: CreateMagicLinkToken :one
INSERT INTO magic_link_tokens (id, token_hash, email, issued_at, expires_at, used_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetMagicLinkTokenByHash :one
SELECT id, token_hash, email, issued_at, expires_at, used_at
FROM magic_link_tokens
WHERE token_hash = $1;

-- name: ConsumeMagicLinkToken :exec
UPDATE magic_link_tokens
SET used_at = $1
WHERE id = $2 AND used_at IS NULL;
