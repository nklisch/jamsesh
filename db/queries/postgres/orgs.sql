-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateProtectedOrg :one
-- Inserts an org row with org_protected=true. Used at startup by the
-- playground provisioning hook for the reserved `playground` org.
-- Slug uniqueness is enforced by the existing UNIQUE constraint on orgs.slug.
INSERT INTO orgs (id, name, slug, session_invite_policy, created_at, org_protected)
VALUES ($1, $2, $3, 'open', $4, TRUE)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at, session_invite_policy, org_protected
FROM orgs
WHERE id = $1;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at, session_invite_policy, org_protected
FROM orgs
WHERE slug = $1;

-- name: GetOrgSessionInvitePolicy :one
SELECT session_invite_policy FROM orgs WHERE id = $1;

-- name: UpdateOrgSessionInvitePolicy :exec
UPDATE orgs SET session_invite_policy = $1 WHERE id = $2;
