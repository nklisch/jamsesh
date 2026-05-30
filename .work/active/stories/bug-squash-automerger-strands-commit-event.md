---
id: bug-squash-automerger-strands-commit-event
kind: story
stage: review
tags: [bug, portal, concurrency, high]
parent: epic-bug-squash-automerger-correctness
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: high
bug_domain: concurrency
bug_location: internal/portal/automerger/worker.go:130
---

# Auto-merger can strand a commit.arrived event so the auto-merge never runs

**Location**: `internal/portal/automerger/worker.go:130` · **Severity**: high · **Pattern**: atomicity violation across multiple atomic ops

The `queues` and `running` maps are mutated without a single consistent lock (the worker's `queues.Delete`/`running.Delete` run outside `w.mu`). When a session worker idles out concurrently with `enqueue`, an interleaving exists where `enqueue` misses the deleted queue under `w.mu`, creates a fresh channel and buffers the event, then `ensureSessionWorker` sees `running` still set and does NOT spawn a worker. The old worker then clears `running`, leaving a buffered commit.arrived event with no draining goroutine — the auto-merge never runs unless another push happens to arrive for that session. Fix: make queue+worker lifecycle a single atomic decision under one mutex (collapse `queues`+`running` into one per-session struct, or perform both deletes and the `running` check under `w.mu`).

```go
w.mu.Lock(); raw, exists := w.queues.Load(e.SessionID); ... w.mu.Unlock()
case ch <- e: w.ensureSessionWorker(ctx, e.SessionID, ch)
// idle path (NOT under w.mu): case <-idle.C: w.queues.Delete(sessionID); return
```

## Implementation notes

Replaced the `queues` + `running` `sync.Map` pair and `ensureSessionWorker` with a single
`w.mu`-guarded `map[string]*sessionQueue`. Membership in the map is the "a goroutine is
draining" flag — no separate sentinel needed.

Key design points honoured:
- `enqueue` creates the `sessionQueue`, inserts it into the map, and spawns the goroutine all
  under `w.mu`. Pushing is also under the lock (non-blocking, buffered + `default`), so the
  idle-exit race window is eliminated.
- The idle case re-checks `len(sq.ch) == 0` under `w.mu` before deleting and exiting. If
  events arrived during the timer-fire → lock-acquire window, it resets the timer and keeps
  draining.
- `onIdleDecision func(sessionID string, willExit bool)` hook (unexported, nil in production)
  is called under `w.mu` just after the re-check, enabling deterministic race testing from the
  internal test package.
- `Start` initialises `w.sessions = make(map[string]*sessionQueue)`.
- `processSessionQueue` ctx-cancellation path deletes from the map so `Stop` + restart
  works cleanly.

Tests added:
- `worker_race_test.go` (package `automerger_test`): `TestWorkerRace_NoStrandedEvents` —
  4 sessions × 10 events, IdleTimeout=1ms, asserts all 40 events produce outcomes under
  `-race`.
- `worker_race_internal_test.go` (package `automerger`): `TestWorkerRace_DeterministicIdleRace` —
  uses `onIdleDecision` hook to fire a second enqueue while the idle decision is live, asserts
  both events are processed.
