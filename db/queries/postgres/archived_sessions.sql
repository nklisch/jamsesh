-- name: InsertArchivedSession :exec
INSERT INTO archived_sessions (session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetArchivedSession :one
SELECT session_id, org_id, name, goal_text, member_account_ids, ended_at, archived_at, end_reason, final_branch_name
FROM archived_sessions
WHERE org_id = $1 AND session_id = $2;
