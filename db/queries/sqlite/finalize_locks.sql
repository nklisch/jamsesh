-- name: InsertFinalizeLock :exec
INSERT INTO finalize_locks (
    id, org_id, session_id, acquired_by_account_id,
    acquired_at, last_activity_at, selected_commit_shas,
    target_branch, base_sha, mode, commit_message,
    superseded_by_lock_id, released_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetFinalizeLockByID :one
SELECT id, org_id, session_id, acquired_by_account_id,
       acquired_at, last_activity_at, selected_commit_shas,
       target_branch, base_sha, mode, commit_message,
       superseded_by_lock_id, released_at
FROM finalize_locks
WHERE id = ?;

-- name: GetActiveFinalizeLockForSession :one
SELECT id, org_id, session_id, acquired_by_account_id,
       acquired_at, last_activity_at, selected_commit_shas,
       target_branch, base_sha, mode, commit_message,
       superseded_by_lock_id, released_at
FROM finalize_locks
WHERE session_id = ?
  AND released_at IS NULL
  AND superseded_by_lock_id IS NULL
ORDER BY acquired_at DESC
LIMIT 1;

-- name: UpdateFinalizeLockCuration :exec
UPDATE finalize_locks
SET selected_commit_shas = ?,
    target_branch = ?,
    base_sha = ?,
    mode = ?,
    commit_message = ?,
    last_activity_at = ?
WHERE id = ?;

-- name: TouchFinalizeLock :exec
UPDATE finalize_locks
SET last_activity_at = ?
WHERE id = ?;

-- name: ReleaseFinalizeLock :exec
UPDATE finalize_locks
SET released_at = ?
WHERE id = ? AND released_at IS NULL;

-- name: ReleaseFinalizeLockIfStale :execrows
UPDATE finalize_locks
SET released_at = ?
WHERE id = ?
  AND released_at IS NULL
  AND last_activity_at < ?;

-- name: SupersedeFinalizeLock :exec
UPDATE finalize_locks
SET superseded_by_lock_id = ?
WHERE id = ?;
