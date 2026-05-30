---
id: bug-squash-pghandle-heartbeat-conn-race
kind: story
stage: drafting
tags: [bug, portal, concurrency]
parent: epic-bug-squash-worker-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: concurrency
bug_location: internal/portal/lease/postgres.go:219
---

# pgHandle.Release races the heartbeat goroutine on the same *sql.Conn

**Location**: `internal/portal/lease/postgres.go:219` · **Severity**: medium · **Pattern**: cancellation leaving partial state / unsynchronized shared resource

`Release` closes `h.done` then immediately uses `h.conn` (advisory unlock, MarkLeaseReleased, `conn.Close()`), but does not wait for the heartbeat goroutine to acknowledge `done`. If the ticker has already fired and the goroutine is mid-`PingContext` when Release runs `conn.Close()`, two goroutines operate on one `*sql.Conn` concurrently — `database/sql` does not guarantee that is safe, risking pooled-connection corruption or a panic. Fix: have the heartbeat goroutine close a `heartbeatDone` channel on return and have Release wait on it before touching the conn, or serialize all conn access behind a pgHandle mutex.

```go
func (h *pgHandle) Release() error {
    h.once.Do(func() {
        close(h.done)
        _, _ = h.conn.ExecContext(ctx, "SELECT pg_advisory_unlock(...)")
        h.conn.Close()  // races an in-flight heartbeat PingContext
    })
}
```
