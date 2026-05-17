-- name: UpsertPresence :exec
INSERT INTO presence (org_id, session_id, account_id, ref, current_sha, last_active_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(session_id, account_id, ref) DO UPDATE SET
    current_sha = EXCLUDED.current_sha,
    last_active_at = EXCLUDED.last_active_at;

-- name: ListPresenceForSession :many
SELECT org_id, session_id, account_id, ref, current_sha, last_active_at
FROM presence
WHERE session_id = $1
ORDER BY account_id, ref;
