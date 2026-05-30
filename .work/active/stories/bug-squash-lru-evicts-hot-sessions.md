---
id: bug-squash-lru-evicts-hot-sessions
kind: story
stage: drafting
tags: [bug, portal, concurrency]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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
