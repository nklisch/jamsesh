-- name: CreateOrg :one
INSERT INTO orgs (id, name, slug, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetOrgByID :one
SELECT id, name, slug, created_at
FROM orgs
WHERE id = $1;

-- name: GetOrgBySlug :one
SELECT id, name, slug, created_at
FROM orgs
WHERE slug = $1;
