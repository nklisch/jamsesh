-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at, session_invite_policy
FROM orgs
WHERE id = ?;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at, session_invite_policy
FROM orgs
WHERE slug = ?;

-- name: GetOrgSessionInvitePolicy :one
SELECT session_invite_policy FROM orgs WHERE id = ?;

-- name: UpdateOrgSessionInvitePolicy :exec
UPDATE orgs SET session_invite_policy = ? WHERE id = ?;
