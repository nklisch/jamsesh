-- name: InsertSessionInvite :one
INSERT INTO session_invites (id, org_id, session_id, inviter_account_id, invitee_email, token_hash, created_at, expires_at, accepted_at, accepted_by_account_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSessionInviteByID :one
SELECT * FROM session_invites WHERE id = ?;

-- name: GetSessionInviteByTokenHash :one
SELECT * FROM session_invites WHERE token_hash = ?;

-- name: MarkSessionInviteAccepted :exec
UPDATE session_invites
SET accepted_at = ?, accepted_by_account_id = ?
WHERE id = ? AND accepted_at IS NULL;

-- name: ListPendingSessionInvitesForSession :many
SELECT * FROM session_invites
WHERE session_id = ? AND accepted_at IS NULL AND expires_at > ?
ORDER BY created_at ASC;
