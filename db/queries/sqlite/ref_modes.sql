-- name: UpsertRefMode :exec
INSERT INTO ref_modes (session_id, ref, mode)
VALUES (?, ?, ?)
ON CONFLICT (session_id, ref) DO UPDATE SET mode = excluded.mode;

-- name: GetRefMode :one
SELECT session_id, ref, mode
FROM ref_modes
WHERE session_id = ? AND ref = ?;

-- name: ListRefModesForSession :many
SELECT session_id, ref, mode
FROM ref_modes
WHERE session_id = ?
ORDER BY ref ASC;
