-- name: InsertConflictEvent :exec
INSERT INTO conflict_events (
    id, org_id, session_id, source_commit, draft_tip, ancestor,
    conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetConflictEventByID :one
SELECT id, org_id, session_id, source_commit, draft_tip, ancestor,
       conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
FROM conflict_events
WHERE id = $1;

-- name: MarkConflictEventResolved :exec
UPDATE conflict_events
SET status = 'resolved',
    resolving_commit_sha = $1,
    resolved_at = $2
WHERE id = $3 AND session_id = $4 AND status = 'open';

-- name: ListOpenConflictEventsForSession :many
SELECT id, org_id, session_id, source_commit, draft_tip, ancestor,
       conflicts, addressed_to, status, resolving_commit_sha, created_at, resolved_at
FROM conflict_events
WHERE session_id = $1 AND status = 'open'
ORDER BY created_at ASC;
