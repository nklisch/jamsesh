-- name: CreateOAuthToken :one
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetOAuthTokenByHash :one
SELECT id, account_id, token_hash, kind, session_id, issued_at, expires_at, last_used_at, revoked_at
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
SELECT id, account_id, token_hash, kind, session_id, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE account_id = $1
ORDER BY issued_at DESC;

-- name: CreateAnonymousBearer :one
-- Inserts an anonymous-session-scoped bearer row. The session_id FK
-- ensures the bearer is cascade-deleted when the session is destroyed
-- (ON DELETE CASCADE). expires_at is set by the caller, typically to
-- the session's hard-cap deadline.
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id,
                          issued_at, expires_at)
VALUES ($1, $2, $3, 'anonymous_session_bearer', $4, $5, $6)
RETURNING *;

-- name: RevokeBearersForSession :exec
-- Marks every bearer (any kind) associated with a session as revoked.
-- Used by the session destruction routine in session-lifecycle feature
-- as the first step of the cascade: revoke bearers, delete dependent
-- rows, delete session row, cascade.
UPDATE oauth_tokens
   SET revoked_at = $1
 WHERE session_id = $2
   AND revoked_at IS NULL;
