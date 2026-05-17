-- name: EnsureEventSeqRow :exec
INSERT INTO event_seq (session_id, next) VALUES (?, 0)
ON CONFLICT(session_id) DO NOTHING;

-- name: AllocateNextSeq :one
UPDATE event_seq SET next = next + 1 WHERE session_id = ? RETURNING next;

-- name: AllocateNextSeqN :one
UPDATE event_seq SET next = next + ? WHERE session_id = ? RETURNING next;

-- name: InsertEvent :exec
INSERT INTO events (id, org_id, session_id, seq, type, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListEventsSince :many
SELECT id, org_id, session_id, seq, type, payload, created_at
FROM events
WHERE session_id = ? AND seq > ?
ORDER BY seq ASC
LIMIT ?;
