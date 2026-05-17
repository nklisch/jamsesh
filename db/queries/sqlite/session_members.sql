-- name: AddSessionMember :exec
INSERT INTO session_members (org_id, session_id, account_id, role, joined_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSessionMember :one
SELECT org_id, session_id, account_id, role, joined_at
FROM session_members
WHERE org_id = ? AND session_id = ? AND account_id = ?;

-- name: ListSessionMembers :many
SELECT org_id, session_id, account_id, role, joined_at
FROM session_members
WHERE org_id = ? AND session_id = ?
ORDER BY joined_at ASC;

-- name: RemoveSessionMember :exec
DELETE FROM session_members
WHERE org_id = ? AND session_id = ? AND account_id = ?;

-- name: ListSessionMembershipsForAccount :many
-- Intentional cross-org exception: returns sessions across all orgs for the
-- authenticated account. The caller receives org_id on each row so it can
-- route further org-scoped queries correctly. This is the only query in this
-- file that does not restrict by org_id in WHERE.
SELECT sm.org_id, sm.session_id, sm.account_id, sm.role, sm.joined_at,
       s.name AS session_name, s.status AS session_status, s.goal AS session_goal
FROM session_members sm
JOIN sessions s ON s.id = sm.session_id
WHERE sm.account_id = ?
ORDER BY sm.joined_at DESC;
