-- name: InsertOrgInvite :one
INSERT INTO org_invites (id, org_id, inviter_account_id, recipient_email, token_hash, created_at, expires_at, accepted_at, accepted_by_account_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOrgInviteByID :one
SELECT * FROM org_invites WHERE id = ?;

-- name: GetOrgInviteByTokenHash :one
SELECT * FROM org_invites WHERE token_hash = ?;

-- name: MarkOrgInviteAccepted :exec
UPDATE org_invites
SET accepted_at = ?, accepted_by_account_id = ?
WHERE id = ? AND accepted_at IS NULL;

-- name: ListPendingOrgInvitesForOrg :many
SELECT * FROM org_invites
WHERE org_id = ? AND accepted_at IS NULL AND expires_at > ?
ORDER BY created_at ASC;

-- name: ListPendingOrgInvitesForEmail :many
SELECT * FROM org_invites
WHERE recipient_email = ? AND accepted_at IS NULL AND expires_at > ?
ORDER BY created_at ASC;
