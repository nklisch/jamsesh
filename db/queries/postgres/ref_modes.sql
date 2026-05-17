-- name: UpsertRefMode :exec
INSERT INTO ref_modes (session_id, ref, mode)
VALUES ($1, $2, $3)
ON CONFLICT (session_id, ref) DO UPDATE SET mode = excluded.mode;

-- name: GetRefMode :one
SELECT session_id, ref, mode
FROM ref_modes
WHERE session_id = $1 AND ref = $2;

-- name: ListRefModesForSession :many
SELECT session_id, ref, mode
FROM ref_modes
WHERE session_id = $1
ORDER BY ref ASC;
