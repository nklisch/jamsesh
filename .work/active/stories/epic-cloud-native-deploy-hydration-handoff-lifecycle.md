---
id: epic-cloud-native-deploy-hydration-handoff-lifecycle
kind: story
stage: implementing
tags: [portal]
parent: epic-cloud-native-deploy-hydration-handoff
depends_on: [epic-cloud-native-deploy-hydration-handoff-hydrator]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration + Handoff — LifecycleManager

## Scope

The `LifecycleManager` that ties everything together: acquire-hydrate on
first request, release-evict on lease loss / explicit release / idle
timeout / LRU cap. One per pod. Owns per-session lease handles and the
local-cache lifecycle.

Implements **Unit 2** of `epic-cloud-native-deploy-hydration-handoff`.
See parent feature body for full specs + acceptance criteria.

## Files

New:
- `internal/portal/storage/objectstore/lifecycle.go` — `LifecycleManager`, `sessionEntry`
- `internal/portal/storage/objectstore/lifecycle_test.go`

## Acceptance criteria

- [ ] AcquireForRequest first-time → hydrates + returns handle
- [ ] AcquireForRequest second-time same session → returns same handle, no re-hydration
- [ ] AcquireForRequest on `lease.ErrAlreadyHeld` → wrapped error (caller 503s)
- [ ] AcquireForRequest on hydration failure → releases lease + returns error
- [ ] `handle.Lost()` closing triggers automatic Release
- [ ] Release waits for in-flight Syncer uploads (test with blockingBackend)
- [ ] Release evicts local cache (verify directory removed)
- [ ] Idle eviction releases sessions idle > IdleTimeout
- [ ] LRU eviction releases oldest-lastActive when CacheMaxBytes exceeded
- [ ] Shutdown (ctx cancel) releases all active leases within 30s
- [ ] AcquireForRequest racing with Release on the same session waits or retries (no double-state)

## Notes

- `sessionEntry` struct holds: orgID, handle, acquiredAt, `lastActiveAt atomic.Pointer[time.Time]`, `releasing atomic.Bool`.
- Per-session state in `sync.Map[string]*sessionEntry`.
- `OrgIDLookup func(ctx, sessionID) (string, error)` — populated by caller in main.go (queries the Store).
- Release ordering: CAS releasing → wait for in-flight uploads (Syncer's per-session counter; 10s bounded) → handle.Release() → os.RemoveAll(repoPath) → delete sync.Map entry.
- LRU bytes tracking: refresh per-session repo size on each acquire/release (sum into a cumulative gauge); when sum > CacheMaxBytes, find oldest-lastActiveAt session and Release.
- Shutdown: cancel idle goroutine ctx → parallel-Release all entries in goroutines with 30s per-session bounded wait.
- The Syncer's `SyncPushPath` signature changes in Unit 3 to accept an existing handle — LifecycleManager doesn't call SyncPushPath itself; the postreceive Emitter does that after calling LifecycleManager.AcquireForRequest.
