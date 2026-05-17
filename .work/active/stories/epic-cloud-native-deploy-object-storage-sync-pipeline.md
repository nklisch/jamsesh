---
id: epic-cloud-native-deploy-object-storage-sync-pipeline
kind: story
stage: implementing
tags: [portal]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-backend, epic-cloud-native-deploy-object-storage-sync-manifest]
release_binding: null
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
