-- name: UpsertPresence :exec
INSERT INTO presence (org_id, session_id, account_id, ref, current_sha, last_active_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id, account_id, ref) DO UPDATE SET
    current_sha = excluded.current_sha,
    last_active_at = excluded.last_active_at;

-- name: ListPresenceForSession :many
SELECT org_id, session_id, account_id, ref, current_sha, last_active_at
FROM presence
WHERE session_id = ?
ORDER BY account_id, ref;
