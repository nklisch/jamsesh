-- name: InsertOAuthState :exec
INSERT INTO oauth_state (nonce, provider, redirect_uri, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeOAuthState :one
DELETE FROM oauth_state WHERE nonce = $1 RETURNING *;

-- name: CleanupExpiredOAuthState :exec
DELETE FROM oauth_state WHERE expires_at < $1;
