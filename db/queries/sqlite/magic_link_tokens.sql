-- name: CreateMagicLinkToken :one
INSERT INTO magic_link_tokens (id, token_hash, email, issued_at, expires_at, used_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMagicLinkTokenByHash :one
SELECT id, token_hash, email, issued_at, expires_at, used_at
FROM magic_link_tokens
WHERE token_hash = ?;

-- name: ConsumeMagicLinkToken :execrows
UPDATE magic_link_tokens
SET used_at = ?
WHERE id = ? AND used_at IS NULL;
