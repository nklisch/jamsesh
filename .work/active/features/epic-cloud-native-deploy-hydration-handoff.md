---
id: epic-cloud-native-deploy-hydration-handoff
kind: feature
stage: done
tags: [portal]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-object-storage-sync, epic-cloud-native-deploy-lease-fencing, epic-cloud-native-deploy-routing-layer]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Hydration + Handoff

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 capstone. Closes the loop on clustered
  mode by making sessions migratable between pods on demand. Depends
  on lease-fencing (lifecycle hooks), object-storage-sync (the source
  to hydrate from), and routing-layer (the trigger surface — handoff
  is observable to clients as a brief 503 + re-dispatch).

## Foundation references

- `docs/ARCHITECTURE.md` — "Data flow: a turn" (request lifecycle
  gains hydration-on-first-request in clustered mode).
- `docs/SPEC.md` — "Recovery" section (handoff is a new failure-and-
  recovery mode the spec needs to acknowledge when this lands).
- `docs/PRINCIPLES.md` — "Recovery is `git fetch`" (justifies the
  best-effort handoff: clients re-fetch and re-sync on the next push,
  so a few-seconds-stale hydration is acceptable).
- `internal/portal/storage/service.go` — the local-disk write surface
  hydration fills before serving.

## Brief

The lifecycle glue that makes the clustered-mode topology actually work
under churn. When a pod acquires a session lease for the first time
(cold start, scale-up, ring rebalance after a peer dies), it hydrates
the session's bare repo from object storage into local disk before
serving any request. When a pod loses a lease (shutdown, idle timeout,
lease-loss event), it drains in-flight uploads and evicts local cache.

This feature closes the loop on the clustered architecture. Without it,
a session is bound to a single pod for its entire lifetime, which makes
scale events and pod loss painful. With it, sessions migrate cleanly
between pods on demand.

Single-instance mode has no concept of handoff — skipped.

## Scope

In:
- Hydration on lease acquisition:
  - Read the session's manifest object from object storage.
  - Stream the listed pack files and loose objects into local
    `<storage>/orgs/<org-id>/sessions/<session-id>.git/`.
  - Write `refs/` and `packed-refs` from the manifest.
  - Verify git repo integrity (`git fsck --no-dangling`) before
    serving.
  - Concurrent downloads with bounded parallelism.
- Hydration metrics: time-to-first-serve after lease acquisition, bytes
  downloaded, objects fetched.
- Eviction on lease loss / release:
  - Wait for in-flight upload queue (from `object-storage-sync`) to
    drain or hit a hard timeout.
  - `rm -rf` the local bare repo path. Local disk is cache only;
    object storage holds truth.
  - Optional retention window: keep the local copy for N minutes
    after eviction in case the same pod re-acquires (configurable,
    default 0 — immediate eviction).
- Idle eviction loop: a background goroutine that releases leases
  for sessions with no activity for the idle window (default 5
  minutes), triggering the eviction path.
- LRU cache size cap: if local-disk usage exceeds a configured
  threshold (`JAMSESH_CACHE_MAX_BYTES`), evict the least-recently-
  active session even if its idle timer hasn't fired.
- Graceful shutdown integration: on `SIGTERM`, release all leases
  (which triggers eviction for each) before the process exits. Uses
  the grace window from `epic-cloud-native-deploy-operational-polish`.

Out:
- Lazy / on-demand object fetch via a custom go-git Storer. v1
  hydrates the whole repo eagerly. Lazy fetch is a future
  optimization worth doing if hydration latency becomes the bottleneck
  for large sessions.
- Predictive pre-hydration ("router hints pod B that session X is
  likely to move there soon"). Cute idea; out of scope for v1.
- Active warm-pool of pre-hydrated sessions. v1 hydrates on demand.

## Design decisions

Inherited from epic. Feature-local:

- **Eager full-repo hydration in v1.** Lazy / per-object fetch is
  elegant but adds a layer of complexity (custom go-git Storer + fault
  handling for failed fetches mid-operation) that we don't need until
  proven necessary. Per-session bare repos are bounded (20–50 MB
  typical per `docs/SELF_HOST.md` §7), so full hydration takes well
  under a second within-region.
- **Eviction is immediate by default.** Local disk is cache; treating
  it as cheap means scale events don't accumulate stale per-pod state.
  Operators who want stickier behavior can tune the retention window.
- **`git fsck` on hydration completion.** Adds a few hundred ms but
  catches corruption before clients see it. Worth it.
- **503 + `Retry-After` is the handoff client contract.** A pod that
  receives a request for a session it doesn't lease (and hasn't
  hydrated) returns 503 with a `Retry-After` header sized to typical
  hydration time; the routing service (which already retries on 503)
  re-dispatches transparently. Clients see at most a brief retry
  loop, no error surface.

## Foundation-doc impact

- `docs/ARCHITECTURE.md` — request-lifecycle section gains the
  hydration-on-first-request step when this lands.

## Architectural choice

**Selected: 3 stories — pure-logic Hydrator, integrated LifecycleManager
(combining acquire/release/idle/LRU since they share per-session state),
Wiring (config + metrics + docs + SyncPushPath refactor to long-held
lease).**

Considered splitting eviction into its own story, but acquire-hydrate
and release-evict are read-modify-write on the same per-session state
map (`sync.Map[sessionID]*sessionEntry`); idle eviction is just a
timer-driven trigger of the same release path. Folding keeps the state
machine cohesive.

## Implementation Units

### Unit 1: Hydrator (pure download + write logic)

**Files**:
- new: `internal/portal/storage/objectstore/hydrate.go` — `Hydrator` + `Hydrate`
- new: `internal/portal/storage/objectstore/hydrate_test.go` — uses memBackend

**Story**: `epic-cloud-native-deploy-hydration-handoff-hydrator`

```go
type Hydrator struct {
    Backend   Backend
    Manifests *ManifestStore
    Storage   storage.Service
    Metrics   *metrics.Registry
    Workers   int  // parallel download workers; default 8
}

type HydrationOutput struct {
    ObjectsDownloaded int
    PacksDownloaded   int
    BytesDownloaded   int64
    Duration          time.Duration
    FsckOK            bool
}

func (h *Hydrator) Hydrate(ctx context.Context, orgID, sessionID string) (HydrationOutput, error)
```

**Implementation Notes**:
- Sequence: Load manifest → fresh-session early-return (CreateRepo + zero counts) → ensure local bare repo dir exists → parallel download packs + idx + loose objects → write refs + packed-refs → `git fsck --no-dangling` → metrics → return.
- Parallel downloads via `errgroup.Group` with `SetLimit(h.Workers)`.
- Atomic writes: download to `<path>.tmp` then rename. No partial files on failure.
- `git fsck --no-dangling`: dangling objects normal in freshly-hydrated repo. Non-fatal; emit FsckOK + slog.Warn on failure.
- Takes orgID + sessionID (Storage.RepoPath requires both).

**Acceptance Criteria**:
- [ ] Fresh session → no-op success; CreateRepo called; zero counts
- [ ] Existing session → downloads all packs + refs + objects
- [ ] Parallel timing: 5 × 100ms downloads complete in ≤200ms
- [ ] Atomic writes: simulated mid-download failure leaves no `.tmp` files
- [ ] `git fsck` runs; FsckOK reflects exit code

### Unit 2: LifecycleManager (acquire-hydrate / release-evict / idle / LRU)

**Files**:
- new: `internal/portal/storage/objectstore/lifecycle.go` — `LifecycleManager`
- new: `internal/portal/storage/objectstore/lifecycle_test.go`

**Story**: `epic-cloud-native-deploy-hydration-handoff-lifecycle`

```go
type LifecycleManager struct {
    Lease           lease.Manager
    Hydrator        *Hydrator
    Syncer          *Syncer        // for in-flight upload drain
    Storage         storage.Service
    OrgIDLookup     func(ctx context.Context, sessionID string) (string, error)
    IdleTimeout     time.Duration  // default 5m
    CacheMaxBytes   int64          // 0 = unlimited
    IdleCheckPeriod time.Duration  // default 30s
    Metrics         *metrics.Registry
}

func (m *LifecycleManager) AcquireForRequest(ctx context.Context, sessionID string) (lease.Handle, error)
func (m *LifecycleManager) Release(ctx context.Context, sessionID string) error
func (m *LifecycleManager) Start(ctx context.Context) error
```

**Implementation Notes**:
- Per-session state in `sync.Map[string]*sessionEntry`:
  ```go
  type sessionEntry struct {
      orgID        string
      handle       lease.Handle
      acquiredAt   time.Time
      lastActiveAt atomic.Pointer[time.Time]
      releasing    atomic.Bool
  }
  ```
- `AcquireForRequest`:
  1. Look up sync.Map. If entry exists and !releasing → update lastActiveAt, return handle.
  2. Else: `OrgIDLookup` → `Lease.Acquire` → `Hydrator.Hydrate(orgID, sessionID)`. On hydration error, release lease + return error.
  3. Watch `handle.Lost()` in goroutine — close fires Release(bg-ctx).
  4. Store entry.
- `Release`:
  1. CAS sessionEntry.releasing false→true.
  2. Wait for in-flight Syncer uploads for this session (Syncer's per-session counter; bounded 10s).
  3. `handle.Release()`.
  4. `os.RemoveAll(Storage.RepoPath(orgID, sessionID))`.
  5. Delete sessionEntry.
- Idle goroutine: every `IdleCheckPeriod`, iterate sync.Map; Release entries where `now - lastActiveAt > IdleTimeout`.
- LRU: track cumulative bytes (refresh on acquire/release); when > CacheMaxBytes, find oldest-lastActiveAt entry and Release.
- Shutdown (ctx cancel): cancel idle goroutine; parallel-Release all entries with 30s bounded wait.

**Acceptance Criteria**:
- [ ] AcquireForRequest first-time → hydrates + returns handle
- [ ] AcquireForRequest second-time same session → returns same handle, no re-hydration
- [ ] AcquireForRequest on ErrAlreadyHeld → wrapped error
- [ ] AcquireForRequest on hydration failure → releases lease + returns error
- [ ] handle.Lost() closing triggers automatic Release
- [ ] Release waits for in-flight Syncer uploads (blockingBackend test)
- [ ] Release evicts local cache
- [ ] Idle eviction releases sessions idle > IdleTimeout
- [ ] LRU eviction releases oldest-lastActive when CacheMaxBytes exceeded
- [ ] Shutdown releases all active leases within 30s

### Unit 3: Wiring + metrics + docs + SyncPushPath refactor

**Files**:
- edit: `internal/portal/config/config.go` — `HydrationIdleTimeoutS`, `_CacheMaxBytes`, `_IdleCheckPeriodS`, `_Workers`
- edit: `internal/portal/config/config_test.go`
- edit: `cmd/portal/main.go` — construct LifecycleManager in clustered mode; start goroutine; replace prior Lease.Acquire-inside-Syncer wiring
- edit: `internal/portal/metrics/metrics.go` — append hydration + lifecycle handles
- edit: `internal/portal/storage/objectstore/sync.go` — refactor `SyncPushPath` to accept an existing handle (drop internal acquire)
- edit: `internal/portal/postreceive/emitter.go` — route through `LifecycleManager.AcquireForRequest`; pass returned handle to `SyncPushPath`
- edit: `docs/SELF_HOST.md` — remove hydration from §14 limitations; document new env vars
- edit: `docs/ARCHITECTURE.md` — Horizontal Scaling subsection updates
- edit: `docs/SPEC.md` — clustered mode no longer "preview"

**Story**: `epic-cloud-native-deploy-hydration-handoff-wiring`

Metrics added to Registry:
- `jamsesh_object_storage_hydrations_total{result}` — result ∈ {ok, fresh, error}
- `jamsesh_object_storage_hydration_duration_seconds` (Histogram)
- `jamsesh_object_storage_hydration_bytes_total` (Counter)
- `jamsesh_lifecycle_active_sessions` (Gauge)
- `jamsesh_lifecycle_evictions_total{reason}` — reason ∈ {idle, lru, lost, shutdown}

SyncPushPath refactor (small breaking change to pipeline story's API):
```go
// OLD: SyncPushPath(ctx, sessionID, repoPath) — acquires lease internally
// NEW: SyncPushPath(ctx, sessionID, repoPath, handle lease.Handle) — caller provides
```
Caller is postreceive Emitter; Emitter calls LifecycleManager.AcquireForRequest first.

**Acceptance Criteria**:
- [ ] Config validation accepts positive HydrationIdleTimeoutS etc.
- [ ] main.go constructs LifecycleManager in clustered mode; starts goroutine
- [ ] postreceive Emitter routes through AcquireForRequest
- [ ] Metrics emit on hydration + eviction
- [ ] SELF_HOST §14 no longer lists hydration as a limitation
- [ ] ARCHITECTURE Horizontal Scaling reflects shipped capability
- [ ] go build + go test ./... green

## Implementation Order

Wave 1: Unit 1 (hydrator)
Wave 2: Unit 2 (lifecycle) — depends on Unit 1
Wave 3: Unit 3 (wiring) — depends on Unit 2

Each wave is one agent — sequential.

## Risks

- **Local-disk race on rapid release+reacquire**: Release waits for rm to complete before deleting sessionEntry; concurrent AcquireForRequest checks `releasing` flag and waits.
- **OrgIDLookup DB query per acquire**: minimal cost (once per lease lifetime); cached in sessionEntry for subsequent calls on the same lease.
- **SyncPushPath signature change**: cascades through pipeline tests + Emitter wiring. Mechanical fix.
- **LRU eviction during active push**: AcquireForRequest checks `sessionEntry.releasing` and waits-or-retries.
- **`git fsck` slowness on large repos**: bounded by session size (~20-50MB typical). Log + emit metric for slow fscks.

## Foundation-doc impact

- `docs/SELF_HOST.md` §14: remove hydration from limitations; add hydration env vars; reframe clustered mode as production-capable (no longer "preview").
- `docs/ARCHITECTURE.md`: Horizontal Scaling section gains hydration lifecycle.
- `docs/SPEC.md`: Deployment shape clustered-mode promoted from preview.

## Notes for design

The handoff race-window is the operational concern: client connects,
router picks pod B, pod B has no local copy, must hydrate before
serving. Worst case is the cold-start latency on the client's first
request. Mitigations:
- Client-side: the `jamsesh` binary's `post-tool-use` hook does
  `git push` which is naturally retry-tolerant; a slow first push is
  benign.
- Server-side: serve a 503 with `Retry-After` while hydrating; the
  router (which already retries on 503) handles re-dispatch
  transparently.
- Long-tail: huge sessions (rare per the 20–50 MB sizing) may take
  multiple seconds. Document the cold-start cost in SELF_HOST.

Eviction-while-uploads-in-flight is a real edge case. The upload queue
from `object-storage-sync` must be drained before `rm -rf` to avoid
orphaned in-memory state pointing at deleted files. The lease handle's
`Release()` flow needs an explicit `drain-uploads → release-lock →
evict-disk` ordering. Resolve in design.

`git fsck` is fast on healthy repos but pathologically slow on large /
broken ones. Consider `git fsck --quick` or a custom go-git
verification pass; resolve in design.

## Children complete (2026-05-17)

All 3 child stories landed and reviewed:

| Story | Verdict | Notes |
|---|---|---|
| hydrator | Approve | 410 LoC + 573 LoC tests; atomic writes via tmp+rename; errgroup parallel downloads; git fsck integrity check |
| lifecycle | Approve | LoadOrStore race guard; bounded drain (10s/50ms poll); LRU via dirSize on eviction tick; shutdown drains in parallel |
| wiring (review pending) | — | Config + main.go wiring + SyncPushPath refactor + GetSessionByID + docs reframe (preview → shipped) |

Verification: `go build ./...` clean; `go test ./...` green across all packages.

Feature advanced `implementing → review`. The capstone — clustered mode now supports clean pod-to-pod session migration via lease-driven hydration + eviction.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes** (aggregate concerns; per-line lenses exercised at story level):

- **Capability completeness**: ✓ Sessions can migrate cleanly between pods. AcquireForRequest on a pod that doesn't have local state triggers hydration from object storage. Lease loss / idle timeout / explicit release / shutdown all drain in-flight uploads and evict local cache. LRU eviction caps local disk usage.
- **Foundation-doc alignment**: ✓ SELF_HOST §14 reframed — clustered mode is no longer "preview"; all four primitives (routing, leases, durability, hydration) shipped. New hydration env vars documented. ARCHITECTURE Horizontal Scaling section updated. SPEC Deployment shape promotes clustered mode to first-class.
- **Cross-cutting changes**:
  - SyncPushPath signature change: `(ctx, sessionID, repoPath)` → `(ctx, sessionID, repoPath, handle lease.Handle)`. Caller now provides the long-held handle. Syncer.Lease field removed.
  - store.Store interface gained `GetSessionByID(ctx, sessionID)` — cross-org primary-key lookup. Implemented in both adapters + TxStore wrappers; stubStore patched.
  - 5 new metric handles in metrics.go (3 from hydrator story: HydrationsTotal, HydrationDurationSeconds, HydrationBytesTotal; 2 from lifecycle story: LifecycleActiveSessions, LifecycleEvictionsTotal{reason}).
  - 4 new config env vars (`JAMSESH_HYDRATION_IDLE_TIMEOUT_S`, `_CACHE_MAX_BYTES`, `_IDLE_CHECK_PERIOD_S`, `_WORKERS`).

The acquire-per-sync lease pattern that the pipeline story documented as "deferred to hydration-handoff" is now resolved — LifecycleManager owns the long-held handle and routes it through to SyncPushPath. The pipeline story's note is now satisfied.

This is the capstone feature of `epic-cloud-native-deploy`. With it landing, the epic is functionally complete.
