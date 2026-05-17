-- name: InsertComment :exec
INSERT INTO comments (
    id, org_id, session_id, author_account_id, author_kind,
    anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
    body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);

-- name: GetCommentByID :one
SELECT id, org_id, session_id, author_account_id, author_kind,
       anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
       body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
FROM comments
WHERE id = $1;

-- name: ResolveComment :exec
UPDATE comments
SET resolved_at = $1,
    resolved_by_account_id = $2,
    resolution_note = $3
WHERE id = $4 AND session_id = $5 AND resolved_at IS NULL;

-- name: ListCommentsForSession :many
SELECT id, org_id, session_id, author_account_id, author_kind,
       anchor_commit_sha, anchor_file_path, anchor_line_start, anchor_line_end,
       body, addressed_to, kind, created_at, resolved_at, resolved_by_account_id, resolution_note
FROM comments
WHERE session_id = $1
  AND ($2 = '' OR addressed_to LIKE '%' || $3 || '%')
  AND ($4 = '' OR kind = $5)
  AND ($6 = 0 OR ($7 = 1 AND resolved_at IS NOT NULL) OR ($8 = 2 AND resolved_at IS NULL))
  AND ($9 = '' OR anchor_commit_sha = $10)
  AND ($11 = '' OR anchor_file_path = $12)
  AND created_at < $13
ORDER BY created_at DESC
LIMIT $14;
