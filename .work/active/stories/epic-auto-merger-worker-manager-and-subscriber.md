---
id: epic-auto-merger-worker-manager-and-subscriber
kind: story
stage: review
tags: [portal]
parent: epic-auto-merger-worker
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Auto-Merger Worker — Manager + Subscriber

## Scope

Extend `events.Log` with a Subscribe primitive; build the auto-merger Worker with per-session goroutines + bounded queues + replay-on-startup + idle timeout.

## Units delivered

- `internal/portal/events/log.go` (edit) — add `Subscribe(typeFilter string) (Subscriber, unsubscribe func())`; fan out emits to subscribers (non-blocking)
- `internal/portal/automerger/worker.go` — `Worker` struct + `Start(ctx)` + `Stop(ctx)` + per-session goroutine + queue
- `internal/portal/automerger/worker_test.go` — concurrency tests
- `cmd/portal/main.go` (edit) — construct Worker; call Start at startup; Stop on shutdown
- go.sum updates if needed

## Acceptance Criteria

- [x] `events.Log.Subscribe("commit.arrived")` receives only commit.arrived events
- [x] `Worker.Start(ctx)` spawns dispatch + replay scan; returns
- [x] Sync-mode commit triggers merge + Apply pipeline; draft advances; merge.succeeded emitted
- [x] Isolated-mode commit is skipped (draft unchanged, no event)
- [ ] Replay on startup: pre-populated unprocessed commit gets processed (v1 known gap — see implementation notes)
- [x] Backpressure: queue overflow emits `auto-merger.backpressure` event
- [x] `Worker.Stop(ctx)` waits for in-flight queues to drain
- [x] All tests green

## Notes

- The Subscribe channel buffer is 64. Non-blocking send means slow consumers drop events; the replay-on-startup catches that.
- Per-session worker idle timeout 30s; queue size 256 (both configurable via Worker fields).
- The dispatch goroutine reads from the subscriber channel, routes to per-session queue (creating + starting the worker goroutine if needed).

## Implementation notes

### Replay scan — v1 known gap

The replay-on-startup scan is intentionally skipped in v1 (`replayScan` returns nil immediately). This is a documented limitation:

**Gap**: A session that had `commit.arrived` events arrive during a portal downtime will not be auto-merged until the next push triggers a real live event.

**Manual recovery**: Push a no-op commit on the affected ref. This triggers a fresh `commit.arrived` event via post-receive, which re-activates the worker for that session.

**Why deferred**: Implementing replay correctly requires a "list all active sessions" cross-org query that the Store does not yet have, plus an ancestor-walk via go-git to establish the "already-processed" baseline. The cost is significant for the benefit (downtime recovery), and idempotency via the ancestor check makes a future addition safe. Filed as a known gap per the feature design doc.

### OrgID threading

`events.Event` carries `OrgID` directly, so `processEvent` can call `store.GetSession(orgID, sessionID)` without a secondary presence-table lookup. This is why the Worker does not need a `sessionByID` indirection.

### SQLite test isolation

Worker tests use a named shared-cache in-memory SQLite (`file:worker_test_N?mode=memory&cache=shared`) with `SetMaxOpenConns(1)` to ensure all goroutines (including the worker goroutine) share the same migrated schema. Plain `:memory:` DSN is not safe for concurrent goroutines because `database/sql` may open additional connections to fresh, empty in-memory databases.
