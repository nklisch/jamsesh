-- name: InsertOrgInvite :one
INSERT INTO org_invites (id, org_id, inviter_account_id, recipient_email, token_hash, created_at, expires_at, accepted_at, accepted_by_account_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetOrgInviteByID :one
SELECT * FROM org_invites WHERE id = $1;

-- name: GetOrgInviteByTokenHash :one
SELECT * FROM org_invites WHERE token_hash = $1;

-- name: MarkOrgInviteAccepted :exec
UPDATE org_invites
SET accepted_at = $1, accepted_by_account_id = $2
WHERE id = $3 AND accepted_at IS NULL;

-- name: ListPendingOrgInvitesForOrg :many
SELECT * FROM org_invites
WHERE org_id = $1 AND accepted_at IS NULL AND expires_at > $2
ORDER BY created_at ASC;

-- name: ListPendingOrgInvitesForEmail :many
SELECT * FROM org_invites
WHERE recipient_email = $1 AND accepted_at IS NULL AND expires_at > $2
ORDER BY created_at ASC;
