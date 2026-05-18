-- name: IssueLeaseFencingToken :one
SELECT nextval('jamsesh_lease_fencing_tokens')::bigint AS token;

-- name: InsertLease :one
INSERT INTO leases (session_id, pod_id, fencing_token, acquired_at, released_at, heartbeat_at)
VALUES ($1, $2, $3, now(), NULL, now())
ON CONFLICT (session_id) DO UPDATE
    SET pod_id        = EXCLUDED.pod_id,
        fencing_token = EXCLUDED.fencing_token,
        acquired_at   = now(),
        released_at   = NULL,
        heartbeat_at  = now()
RETURNING session_id, pod_id, fencing_token, acquired_at, released_at, heartbeat_at;

-- name: MarkLeaseReleased :exec
UPDATE leases
SET released_at = now()
WHERE session_id = $1;

-- name: UpdateLeaseHeartbeat :exec
UPDATE leases
SET heartbeat_at = now()
WHERE session_id = $1;

-- name: DeleteReleasedLeasesOlderThan :exec
DELETE FROM leases
WHERE released_at IS NOT NULL
  AND released_at < $1;
