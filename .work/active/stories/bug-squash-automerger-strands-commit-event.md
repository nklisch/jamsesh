---
id: bug-squash-automerger-strands-commit-event
kind: story
stage: drafting
tags: [bug, portal, concurrency, high]
parent: epic-bug-squash
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
