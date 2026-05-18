---
id: epic-cloud-native-deploy-hydration-handoff-lifecycle
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-hydration-handoff
depends_on: [epic-cloud-native-deploy-hydration-handoff-hydrator]
release_binding: v0.1.0
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

## Implementation notes

### Files produced

- `internal/portal/storage/objectstore/lifecycle.go` — `LifecycleManager`,
  `sessionEntry`, `dirSize` helper.
- `internal/portal/storage/objectstore/lifecycle_test.go` — 11 tests covering
  all acceptance criteria; race-detector clean.
- `internal/portal/storage/objectstore/sync.go` — added `InFlightCount(sessionID)
  int64` method on `Syncer` (clean API, reads the per-session atomic via `Load`).
- `internal/portal/metrics/metrics.go` — added `LifecycleActiveSessions` (Gauge)
  and `LifecycleEvictionsTotal{reason}` (CounterVec) with labels idle/lru/lost/
  shutdown/explicit; registered in `New()` and wired in the returned `Registry`.

### Design choices

- **Release drain loop** uses a `context.WithTimeout(10s)` sub-context and polls
  every 50ms. The outer drain context is cancelled on timeout; a `slog.Warn` is
  emitted. Continues to handle.Release() and os.RemoveAll regardless.
- **LoadOrStore race guard**: `acquireNew` uses `sync.Map.LoadOrStore` to handle
  the narrow window where two goroutines race to insert the same session. The
  loser releases its handle and returns the winner's. Verified by
  `TestLifecycle_AcquireWhileReleasing` under `-race`.
- **LRU byte tracking**: repo sizes refreshed in the eviction tick via `dirSize`
  (filepath.Walk). Avoids a circular dependency with Syncer's success path.
- **Metrics**: nil-safe checks at every metric call site consistent with the
  hydrator and sync patterns already in the codebase.
- **watchLost goroutine**: spawned after `LoadOrStore` winner is confirmed, so
  it is always cleaned up by `releaseWithReason` regardless of entry provenance.

### All 11 tests pass; `go test -race ./...` green.

## Notes

- `sessionEntry` struct holds: orgID, handle, acquiredAt, `lastActiveAt atomic.Pointer[time.Time]`, `releasing atomic.Bool`.
- Per-session state in `sync.Map[string]*sessionEntry`.
- `OrgIDLookup func(ctx, sessionID) (string, error)` — populated by caller in main.go (queries the Store).
- Release ordering: CAS releasing → wait for in-flight uploads (Syncer's per-session counter; 10s bounded) → handle.Release() → os.RemoveAll(repoPath) → delete sync.Map entry.
- LRU bytes tracking: refresh per-session repo size on each acquire/release (sum into a cumulative gauge); when sum > CacheMaxBytes, find oldest-lastActiveAt session and Release.
- Shutdown: cancel idle goroutine ctx → parallel-Release all entries in goroutines with 30s per-session bounded wait.
- The Syncer's `SyncPushPath` signature changes in Unit 3 to accept an existing handle — LifecycleManager doesn't call SyncPushPath itself; the postreceive Emitter does that after calling LifecycleManager.AcquireForRequest.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Excellent state-machine design. LoadOrStore race guard handles the narrow window where two goroutines race to insert the same session — loser releases its handle and returns the winner's. Verified under `-race` via `TestLifecycle_AcquireWhileReleasing`.

Release drain (10s bounded ctx + 50ms poll) is the right shape; warns on timeout but proceeds to handle.Release() + os.RemoveAll regardless (failsafe — better to leak some local cache than block shutdown forever).

watchLost goroutine spawned AFTER LoadOrStore winner is confirmed — so cleanup is guaranteed regardless of which goroutine actually entered the map.

LRU byte-tracking refreshes via `dirSize` (filepath.Walk) in the eviction tick — avoids the circular dependency with Syncer's success path that an "update on each sync" approach would introduce. At typical session sizes (20-50MB) the walk is cheap and runs every 30s.

Syncer.InFlightCount accessor added cleanly — uses Load (not LoadOrStore) so it doesn't accidentally create an entry. Right primitive.

11 tests cover every acceptance criterion plus the AcquireWhileReleasing race case. 2 new metric handles (LifecycleActiveSessions gauge, LifecycleEvictionsTotal{reason} counter with idle/lru/lost/shutdown/explicit labels) registered cleanly.
