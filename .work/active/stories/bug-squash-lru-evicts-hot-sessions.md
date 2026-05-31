---
id: bug-squash-lru-evicts-hot-sessions
kind: story
stage: done
tags: [bug, portal, concurrency]
parent: epic-bug-squash-worker-lifecycle
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
bug_origin: scan
bug_severity: medium
bug_domain: concurrency
bug_location: internal/portal/storage/objectstore/lifecycle.go:350
---

# LifecycleManager LRU eviction can release a session that just became active

**Location**: `internal/portal/storage/objectstore/lifecycle.go:350` · **Severity**: medium · **Pattern**: iteration during concurrent mutation / check-then-act

The LRU pass evicts from an `active` snapshot captured during `sessions.Range`. Between snapshot and eviction, `AcquireForRequest` can touch `lastActiveAt` (session back in use), yet the LRU loop still picks it as victim based on the stale `lastActive` and calls `releaseWithReason`. Under cache pressure this evicts in-use sessions, forcing immediate re-hydration (lease re-acquire + full object download) and, in clustered mode, can bounce a lease another pod is about to want. Fix: re-validate each victim's `lastActive`/`releasing` immediately before releasing it, skipping any touched since the snapshot.

```go
m.sessions.Range(func(k, v any) bool {
    entry := v.(*sessionEntry)
    la := entry.lastActive()
    active = append(active, candidate{...})  // stale snapshot used later for LRU eviction
})
```

## Implementation notes

Split `releaseWithReason` into CAS + `releaseClaimed(ctx, sessionID, entry, reason)`.
`releaseClaimed` performs steps 2-5 (drain, handle.Release, RemoveAll, sessions.Delete,
metrics) without re-CAS. The LRU loop now: (1) loads the live entry from `sessions.Load`,
(2) CASes `releasing=false→true`, skipping if already claimed, (3) re-validates
`liveEntry.lastActive().After(victim.lastActive)` and restores the claim
(`releasing.Store(false)`) if the session was touched since the snapshot, (4) calls
`releaseClaimed`. `AcquireForRequest` now double-checks `entry.releasing.Load()` after
`touchLastActive` and retries if an eviction claimed the entry during the touch.
`releaseWithReason` (called by idle eviction, watchLost, shutdown) still CASes internally
and calls `releaseClaimed`. Added `TestLifecycle_LRU_SkipsHotSession` (deterministic
fake-clock test confirming the hot session is not evicted) and
`TestLifecycle_LRU_RaceAcquireEvict` (-race concurrency test). Build/vet/`-race` clean:
`go test -race ./internal/portal/storage/objectstore/...`.
