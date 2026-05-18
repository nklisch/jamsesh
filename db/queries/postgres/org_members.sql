-- name: AddOrgMember :exec
INSERT INTO org_members (org_id, account_id, role, created_at)
VALUES ($1, $2, $3, $4);

-- name: GetOrgMember :one
SELECT org_id, account_id, role, created_at
FROM org_members
WHERE org_id = $1 AND account_id = $2;

-- name: ListOrgsForAccount :many
SELECT o.id, o.name, o.slug, o.created_at, o.session_invite_policy
FROM orgs o
JOIN org_members om ON om.org_id = o.id
WHERE om.account_id = $1
ORDER BY o.created_at ASC;

-- name: ListOrgMembers :many
SELECT a.id, a.email, a.display_name, a.github_user_id, a.created_at,
       om.role, om.created_at AS joined_at
FROM accounts a
JOIN org_members om ON om.account_id = a.id
WHERE om.org_id = $1
ORDER BY om.created_at ASC;

-- name: RemoveOrgMember :exec
DELETE FROM org_members
WHERE org_id = $1 AND account_id = $2;
