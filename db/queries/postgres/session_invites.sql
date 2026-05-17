-- name: InsertSessionInvite :one
INSERT INTO session_invites (id, org_id, session_id, inviter_account_id, invitee_email, token_hash, created_at, expires_at, accepted_at, accepted_by_account_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSessionInviteByID :one
SELECT * FROM session_invites WHERE id = $1;

-- name: GetSessionInviteByTokenHash :one
SELECT * FROM session_invites WHERE token_hash = $1;

-- name: MarkSessionInviteAccepted :exec
UPDATE session_invites
SET accepted_at = $1, accepted_by_account_id = $2
WHERE id = $3 AND accepted_at IS NULL;

-- name: ListPendingSessionInvitesForSession :many
SELECT * FROM session_invites
WHERE session_id = $1 AND accepted_at IS NULL AND expires_at > $2
ORDER BY created_at ASC;
