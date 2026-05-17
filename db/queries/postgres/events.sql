-- name: EnsureEventSeqRow :exec
INSERT INTO event_seq (session_id, next) VALUES ($1, 0)
ON CONFLICT(session_id) DO NOTHING;

-- name: AllocateNextSeq :one
UPDATE event_seq SET next = next + 1 WHERE session_id = $1 RETURNING next;

-- name: AllocateNextSeqN :one
UPDATE event_seq SET next = next + $1 WHERE session_id = $2 RETURNING next;

-- name: InsertEvent :exec
INSERT INTO events (id, org_id, session_id, seq, type, payload, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListEventsSince :many
SELECT id, org_id, session_id, seq, type, payload, created_at
FROM events
WHERE session_id = $1 AND seq > $2
ORDER BY seq ASC
LIMIT $3;

-- name: ListEventsSinceForDigest :many
SELECT id, org_id, session_id, seq, type, payload, created_at
FROM events
WHERE session_id = $1 AND seq > $2
  AND type = ANY(ARRAY[
    'commit.arrived',
    'comment.added',
    'comment.resolved',
    'conflict.detected',
    'conflict.resolved',
    'mode.changed'
  ])
ORDER BY seq ASC
LIMIT $3;
