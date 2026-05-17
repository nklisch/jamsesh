-- name: InsertConflictEvent :exec
INSERT INTO conflict_events (
    id, org_id, session_id, source_commit, draft_tip, ancestor,
    conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetConflictEventByID :one
SELECT id, org_id, session_id, source_commit, draft_tip, ancestor,
       conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
FROM conflict_events
WHERE id = ?;

-- name: MarkConflictEventResolved :exec
UPDATE conflict_events
SET status = 'resolved',
    resolving_commit_sha = ?,
    resolved_at = ?
WHERE id = ? AND session_id = ? AND status = 'open';

-- name: ListOpenConflictEventsForSession :many
SELECT id, org_id, session_id, source_commit, draft_tip, ancestor,
       conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
FROM conflict_events
WHERE session_id = ? AND status = 'open'
ORDER BY created_at ASC;
