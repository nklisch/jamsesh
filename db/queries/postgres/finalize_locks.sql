-- name: InsertFinalizeLock :exec
INSERT INTO finalize_locks (
    id, org_id, session_id, acquired_by_account_id,
    acquired_at, last_activity_at, selected_commit_shas,
    target_branch, base_sha, mode, commit_message,
    superseded_by_lock_id, released_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: GetFinalizeLockByID :one
SELECT id, org_id, session_id, acquired_by_account_id,
       acquired_at, last_activity_at, selected_commit_shas,
       target_branch, base_sha, mode, commit_message,
       superseded_by_lock_id, released_at
FROM finalize_locks
WHERE id = $1;

-- name: GetActiveFinalizeLockForSession :one
SELECT id, org_id, session_id, acquired_by_account_id,
       acquired_at, last_activity_at, selected_commit_shas,
       target_branch, base_sha, mode, commit_message,
       superseded_by_lock_id, released_at
FROM finalize_locks
WHERE session_id = $1
  AND released_at IS NULL
  AND superseded_by_lock_id IS NULL
ORDER BY acquired_at DESC
LIMIT 1;

-- name: UpdateFinalizeLockCuration :exec
UPDATE finalize_locks
SET selected_commit_shas = $1,
    target_branch = $2,
    base_sha = $3,
    mode = $4,
    commit_message = $5,
    last_activity_at = $6
WHERE id = $7;

-- name: TouchFinalizeLock :exec
UPDATE finalize_locks
SET last_activity_at = $1
WHERE id = $2;

-- name: ReleaseFinalizeLock :exec
UPDATE finalize_locks
SET released_at = $1
WHERE id = $2 AND released_at IS NULL;

-- name: SupersedeFinalizeLock :exec
UPDATE finalize_locks
SET superseded_by_lock_id = $1
WHERE id = $2;
