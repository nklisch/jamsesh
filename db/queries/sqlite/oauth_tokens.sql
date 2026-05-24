-- name: CreateOAuthToken :one
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, issued_at, expires_at, last_used_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOAuthTokenByHash :one
SELECT id, account_id, token_hash, kind, session_id, issued_at, expires_at, last_used_at, revoked_at
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
SELECT id, account_id, token_hash, kind, session_id, issued_at, expires_at, last_used_at, revoked_at
FROM oauth_tokens
WHERE account_id = ?
ORDER BY issued_at DESC;

-- name: CreateAnonymousBearer :one
-- Inserts an anonymous-session-scoped bearer row. The session_id FK
-- ensures the bearer is cascade-deleted when the session is destroyed
-- (ON DELETE CASCADE). expires_at is set by the caller, typically to
-- the session's hard-cap deadline.
INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id,
                          issued_at, expires_at)
VALUES (?, ?, ?, 'anonymous_session_bearer', ?, ?, ?)
RETURNING *;

-- name: RevokeBearersForSession :exec
-- Marks every bearer (any kind) associated with a session as revoked.
-- Used by the session destruction routine in session-lifecycle feature
-- as the first step of the cascade: revoke bearers, delete dependent
-- rows, delete session row, cascade.
UPDATE oauth_tokens
   SET revoked_at = ?
 WHERE session_id = ?
   AND revoked_at IS NULL;
