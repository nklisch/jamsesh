-- name: InsertOAuthState :exec
INSERT INTO oauth_state (nonce, provider, redirect_uri, created_at, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: ConsumeOAuthState :one
DELETE FROM oauth_state WHERE nonce = ? RETURNING *;

-- name: CleanupExpiredOAuthState :exec
DELETE FROM oauth_state WHERE expires_at < ?;
