-- name: InsertLease :one
INSERT INTO leases (session_id, pod_id, fencing_token, acquired_at, released_at, heartbeat_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, NULL, CURRENT_TIMESTAMP)
ON CONFLICT (session_id) DO UPDATE
    SET pod_id        = EXCLUDED.pod_id,
        fencing_token = EXCLUDED.fencing_token,
        acquired_at   = CURRENT_TIMESTAMP,
        released_at   = NULL,
        heartbeat_at  = CURRENT_TIMESTAMP
RETURNING session_id, pod_id, fencing_token, acquired_at, released_at, heartbeat_at;

-- name: MarkLeaseReleased :exec
UPDATE leases
SET released_at = CURRENT_TIMESTAMP
WHERE session_id = ?;

-- name: UpdateLeaseHeartbeat :exec
UPDATE leases
SET heartbeat_at = CURRENT_TIMESTAMP
WHERE session_id = ?;
