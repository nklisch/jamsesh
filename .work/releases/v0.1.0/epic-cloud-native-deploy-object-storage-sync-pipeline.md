---
id: epic-cloud-native-deploy-object-storage-sync-pipeline
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-backend, epic-cloud-native-deploy-object-storage-sync-manifest]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object-Storage Sync — Sync pipeline (post-receive integration + fencing)

## Scope

The `Syncer` that enumerates new objects/refs/packs after a push,
uploads them to the Backend, updates the Manifest with conditional-write
semantics, and emits metrics. Triggered SYNCHRONOUSLY from post-receive —
push doesn't ack until sync completes (RPO=0 contract).

Implements **Unit 3** of `epic-cloud-native-deploy-object-storage-sync`.

## Files

New:
- `internal/portal/storage/objectstore/sync.go` — `Syncer` + `SyncPush`
- `internal/portal/storage/objectstore/sync_test.go` — integration

Edit:
- `internal/portal/postreceive/emitter.go` — call `Syncer.SyncPush` after
  event emit when Syncer is non-nil
- `internal/portal/metrics/metrics.go` — append 4 metric handles
  (uploads_total, upload_bytes_total, upload_duration_seconds,
  backpressure_total)

## Acceptance criteria

- [ ] `SyncPush` after a small push uploads all new objects + updated
  refs to S3-compat (MinIO test); returns SyncOutput with counts
- [ ] Two concurrent SyncPushes for different sessions don't interfere
- [ ] SyncPush with a fenced lease (handle token < manifest token)
  returns wrapped `ErrFenced`
- [ ] SyncPush with backpressure (queue full) returns wrapped
  backpressure error
- [ ] Metrics emit on success, fenced, precondition, error, backpressure
- [ ] postreceive Emitter integration: non-nil Syncer → SyncPush called;
  nil Syncer → no-op (single-instance mode)
- [ ] Existing post-receive tests still pass

## Notes

- Per the parent feature design, sync is SYNCHRONOUS — push ack waits for
  upload + manifest save. This is the RPO=0 contract.
- "Enumerate new objects" v1 strategy: walk `objects/xx/*`, check
  existence against manifest's seen-set. First push per session uploads
  all objects (slow but bounded — git repos are typically small);
  subsequent pushes are fast. Track per-session `lastSyncedObjectKey`
  cursor in process memory.
- Pack rewrite detection: list local `objects/pack/*.pack` vs manifest's
  `Packs[]`; upload new packs + idx via PutIdempotent. Old packs in
  manifest but not local: enqueue lazy deletion (don't block ack).
- Backpressure: per-session in-flight upload count via `sync.Map`.
  When > `QueueSize` (default 256), return backpressure error
  → caller 503s with Retry-After.
- `git gc` policy: this story should also call `git config gc.auto 0`
  in `storage.Service.CreateRepo` to disable opportunistic gc. Sidesteps
  the pack-rewrite-mid-push problem. Operators schedule cleanup separately
  if needed. Document in implementation notes.

## Implementation notes

### Acquire-per-sync lease pattern (v1 decision)

`SyncPushPath` acquires and releases a lease internally on every push. In
single-instance mode (`NoopManager`) this is a zero-cost no-op. In clustered
mode it issues a fresh fencing token per push, re-confirming ownership at each
sync boundary. This is safe and correct: fencing tokens are monotonically
increasing, so a later acquire on the same session always gets a token ≥ the
previous one. The long-held lease pattern (acquire once per session lifetime,
reuse across pushes) is the hydration-handoff concern and deferred to the
`hydration-handoff` epic.

### SyncPushPath vs SyncPush naming

The public API is `SyncPushPath(ctx, sessionID, repoPath)`. The explicit
`repoPath` parameter avoids the orgID ambiguity: the Emitter has both
`session.OrgID` and `session.ID` available, so it calls
`Storage.RepoPath(session.OrgID, session.ID)` and passes the result directly.
This sidesteps any orgID-embedding in the Syncer and keeps the wiring in the
caller rather than baking it into the Syncer.

### First-push strategy

On the first push (no manifest yet), `uploadLooseObjects` walks ALL objects in
`objects/xx/*`. This is simple and bounded: git session repos are small (a few
hundred commits at most during an active jamsesh session). Subsequent pushes are
fast because `PutIdempotent` for content-addressed objects is a no-op when the
key already exists with identical content.

### gc.auto 0 rationale

`CreateRepo` now runs `git config gc.auto 0` after `git init --bare`. This
disables opportunistic gc, which would rewrite loose objects into pack files
mid-push. Without this, a push could land between the Syncer's loose-object
walk and pack detection, producing a state where objects appear in a new pack
but were never uploaded as loose objects. With `gc.auto 0`, pack rewrites only
happen from explicit operator `git repack` or `git gc` calls, which run
outside active push windows. Operators should schedule their own gc cadence.

### Lazy pack deletion

When a gc repack produces new pack files, the old packs in the manifest are no
longer on local disk. After saving the new manifest, the Syncer spawns a
goroutine to delete the old pack+idx keys from object storage. This is best-
effort: errors are logged at `slog.Warn` level and do not block push ack.

### Backpressure

Per-session in-flight count is tracked via `sync.Map` of `*int64` atomics.
When the count exceeds `QueueSize` (default 256), `SyncPushPath` returns
`ErrBackpressure` immediately. Callers should propagate this as 503 with a
Retry-After header. The counter is incremented before backpressure check
(so QueueSize=1 allows exactly 1 concurrent call; the 2nd returns backpressure).

### Metrics nil-safety

All metric increments are guarded by `if s.Metrics != nil`. The `Registry`
struct is not nil-safe itself, so the nil check must live at the call site.
This matches the pattern used by `PostgresManager` and the router metrics.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The acquire-per-sync lease pattern is correct for v1 but increases lease-table churn (each push creates a fresh lease row). When hydration-handoff lands and switches to long-held leases, this story's `SyncPushPath` flow becomes simpler — internal acquire becomes a "use existing handle" lookup. Worth noting as a known eventual refactor.
- `SyncPushPath` is the public method name (not `SyncPush` per parent design). The explicit `repoPath` parameter is well-justified in the impl notes — caller has it; passing it dodges orgID-embedding inside Syncer. Reasonable adaptation.

**Notes**: This is the durability layer landing — RPO=0 contract honored. The post-receive integration in `Emitter.EmitForUpdates` is correctly placed AFTER event emission (so internal event log is consistent before external storage is sync'd; on failure both surface together for the caller to 503).

10 tests covering all critical paths: first-push-uploads-all, subsequent-only-new, ref-change, pack-detection, fenced-lease (custom test Manager), concurrent-different-sessions, backpressure (blockingBackend test double), metrics emission, nil-metrics-no-op, empty-repo. Test-double design (memBackend + blockingBackend) is clean — no S3 dependency for the sync flow.

`gc.auto 0` in `CreateRepo` is the right defensive call — sidesteps the pack-rewrite-mid-push race entirely. Operators can still run scheduled `git gc` if disk usage becomes a concern. Documented.

Lazy pack deletion goroutine is best-effort with slog.Warn on failure — appropriate for non-critical cleanup. Push ack isn't blocked.

4 new metric handles added to `Registry` and registered in `New()`. Nil-safe via `if s.Metrics != nil` guards at call sites — matches the established pattern from PostgresManager and router metrics.

Backpressure semantics are documented (QueueSize=1 → exactly 1 concurrent call). Counter using `sync.Map` of `*int64` atomics is the right primitive for high-frequency per-session counting.
