-- name: CreateOAuthToken :one
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetOAuthTokenByHash :one
SELECT id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE token_hash = $1;

-- name: TouchOAuthTokenLastUsed :exec
UPDATE oauth_tokens
SET last_used_at = $1
WHERE id = $2;

-- name: RevokeOAuthToken :exec
UPDATE oauth_tokens
SET revoked_at = $1
WHERE id = $2;

-- name: RevokeAllOAuthTokensForAccount :exec
UPDATE oauth_tokens
SET revoked_at = $1
WHERE account_id = $2 AND revoked_at IS NULL;

-- name: ListOAuthTokensForAccount :many
SELECT id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE account_id = $1
ORDER BY issued_at DESC;
