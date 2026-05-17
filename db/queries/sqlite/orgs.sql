-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at
FROM orgs
WHERE id = ?;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at
FROM orgs
WHERE slug = ?;
