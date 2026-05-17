---
id: epic-auto-merger-worker-manager-and-subscriber
kind: story
stage: implementing
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

- [ ] `events.Log.Subscribe("commit.arrived")` receives only commit.arrived events
- [ ] `Worker.Start(ctx)` spawns dispatch + replay scan; returns
- [ ] Sync-mode commit triggers merge + Apply pipeline; draft advances; merge.succeeded emitted
- [ ] Isolated-mode commit is skipped (draft unchanged, no event)
- [ ] Replay on startup: pre-populated unprocessed commit gets processed
- [ ] Backpressure: queue overflow emits `auto-merger.backpressure` event
- [ ] `Worker.Stop(ctx)` waits for in-flight queues to drain
- [ ] All tests green

## Notes

- The Subscribe channel buffer is 64. Non-blocking send means slow consumers drop events; the replay-on-startup catches that.
- Per-session worker idle timeout 30s; queue size 256 (both configurable via Worker fields).
- The dispatch goroutine reads from the subscriber channel, routes to per-session queue (creating + starting the worker goroutine if needed).
