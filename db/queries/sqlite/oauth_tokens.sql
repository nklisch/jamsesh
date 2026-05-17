-- name: CreateOAuthToken :one
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOAuthTokenByHash :one
SELECT id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE token_hash = ?;

-- name: TouchOAuthTokenLastUsed :exec
UPDATE oauth_tokens
SET last_used_at = ?
WHERE id = ?;

-- name: RevokeOAuthToken :exec
UPDATE oauth_tokens
SET revoked_at = ?
WHERE id = ?;

-- name: RevokeAllOAuthTokensForAccount :exec
UPDATE oauth_tokens
SET revoked_at = ?
WHERE account_id = ? AND revoked_at IS NULL;

-- name: ListOAuthTokensForAccount :many
SELECT id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE account_id = ?
ORDER BY issued_at DESC;
