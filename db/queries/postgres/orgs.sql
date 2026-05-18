-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at, session_invite_policy
FROM orgs
WHERE id = $1;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at, session_invite_policy
FROM orgs
WHERE slug = $1;

-- name: GetOrgSessionInvitePolicy :one
SELECT session_invite_policy FROM orgs WHERE id = $1;

-- name: UpdateOrgSessionInvitePolicy :exec
UPDATE orgs SET session_invite_policy = $1 WHERE id = $2;
