-- name: InsertComment :exec
INSERT INTO comments (
    id, org_id, session_id, author_account_id, author_kind,
    anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
    body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetCommentByID :one
SELECT id, org_id, session_id, author_account_id, author_kind,
       anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
       body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
FROM comments
WHERE id = ?;

-- name: ResolveComment :exec
UPDATE comments
SET resolved_at = ?,
    resolved_by_account_id = ?,
    resolution_note = ?
WHERE id = ? AND session_id = ? AND resolved_at IS NULL;

-- name: ListCommentsForSession :many
SELECT id, org_id, session_id, author_account_id, author_kind,
       anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
       body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
FROM comments
WHERE session_id = ?
  AND (? = '' OR addressed_to LIKE '%' || ? || '%')
  AND (? = '' OR kind = ?)
  AND (? = 0 OR (? = 1 AND resolved_at IS NOT NULL) OR (? = 2 AND resolved_at IS NULL))
  AND (? = '' OR anchor_commit_sha = ?)
  AND (? = '' OR anchor_file_path = ?)
  AND created_at < ?
ORDER BY created_at DESC
LIMIT ?;
