-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: CreateProtectedOrg :one
-- Inserts an org row with org_protected=true. Used at startup by the
-- playground provisioning hook for the reserved `playground` org.
-- Slug uniqueness is enforced by the existing UNIQUE constraint on orgs.slug.
INSERT INTO orgs (id, name, slug, session_invite_policy, created_at, org_protected)
VALUES (?, ?, ?, 'open', ?, 1)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at, session_invite_policy, org_protected
FROM orgs
WHERE id = ?;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at, session_invite_policy, org_protected
FROM orgs
WHERE slug = ?;

-- name: GetOrgSessionInvitePolicy :one
SELECT session_invite_policy FROM orgs WHERE id = ?;

-- name: UpdateOrgSessionInvitePolicy :exec
UPDATE orgs SET session_invite_policy = ? WHERE id = ?;
