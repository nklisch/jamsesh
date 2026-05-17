---
id: epic-cloud-native-deploy-object-storage-sync-wiring
kind: story
stage: review
tags: [portal, documentation]
parent: epic-cloud-native-deploy-object-storage-sync
depends_on: [epic-cloud-native-deploy-object-storage-sync-pipeline]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Object-Storage Sync — Factory + config + main.go wiring + docs

## Scope

Factory that constructs a Backend from a URL scheme. Config additions
for object-storage env vars + validation that clustered mode requires
an object-storage URL. main.go wiring threads Backend → ManifestStore →
Syncer → postreceive Emitter. Foundation-doc updates land here.

Implements **Unit 5** of `epic-cloud-native-deploy-object-storage-sync`.

## Files

New:
- `internal/portal/storage/objectstore/factory.go` — `New(url, cfg) Backend`
- `internal/portal/storage/objectstore/factory_test.go`

Edit:
- `internal/portal/config/config.go` — `JAMSESH_OBJECT_STORAGE_URL`,
  `_REGION`, `_ENDPOINT_URL`, `_PATH_STYLE`, `_SYNC_QUEUE_SIZE`
- `internal/portal/config/config_test.go`
- `cmd/portal/main.go` — wire Backend in clustered mode; nil in single
- `docs/SELF_HOST.md` — object-storage subsection in clustered-mode docs
- `docs/SPEC.md` — storage.Service dual-layer note
- `docs/SECURITY.md` — operator-responsibility row for object-storage IAM
- `docs/ARCHITECTURE.md` — bare-repo dual-layer description

## Acceptance criteria

- [ ] Factory parses `s3://`, `s3-compatible://` (and `gs://`, `azblob://`
  if provider-extensions story has landed); returns error on unknown scheme
- [ ] `cfg.validate()` rejects `DeployMode=clustered` with empty
  `ObjectStorageURL` at startup
- [ ] `cmd/portal/main.go` wires Syncer in clustered mode; passes nil
  Syncer to postreceive Emitter in single-instance mode (preserves
  existing behavior)
- [ ] postreceive Emitter respects nil Syncer (no sync attempted)
- [ ] SELF_HOST.md clustered-mode section documents:
  - All new env vars with defaults
  - Per-provider deploy examples (AWS S3, R2, MinIO; plus GCS/Azure if
    provider-extensions has landed)
  - Cost-model paragraph (~$0.05/active-session/day at heavy use)
  - Required IAM permissions per provider
- [ ] SPEC.md storage.Service section notes the dual-layer mode
- [ ] ARCHITECTURE.md bare-repo storage section documents the dual-layer
  (working cache + object-store truth) model
- [ ] SECURITY.md gains object-storage row in operator-responsibilities

## Notes

- Wiring order in main.go: db.Open → lease.New → metrics.New →
  objectstore.New (when DeployMode=clustered) → ManifestStore →
  Syncer → pass to postreceive Emitter.
- This story does NOT update SELF_HOST.md's existing clustered-mode
  "preview" framing wholesale — it ADDS the object-storage subsection
  documenting what's now shipped. The "limitations" section is updated
  to remove "object-storage-sync is in progress" since this feature is
  shipping; hydration-handoff stays in the limitations list until that
  feature ships.
- Foundation-doc principle: describe AS IT IS NOW, no "previously"
  prose.

## Implementation notes

### Factory (`internal/portal/storage/objectstore/factory.go`)

`New(rawURL, Config)` dispatches on URL scheme:
- `s3://` → `NewS3` directly
- `s3-compatible://` → normalized to `s3://` then `NewS3` (EndpointURL required in Config)
- `gs://` → `NewGCS`
- `azblob://` → `NewAzureBlob`

Unknown schemes return a clear error mentioning "unknown URL scheme". Empty URL
is rejected at parse time.

### Config additions (`internal/portal/config/config.go`)

Five new fields added to `Config` struct with YAML keys and env overlays:
- `ObjectStorageURL` / `JAMSESH_OBJECT_STORAGE_URL`
- `ObjectStorageRegion` / `JAMSESH_OBJECT_STORAGE_REGION`
- `ObjectStorageEndpointURL` / `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL`
- `ObjectStoragePathStyle` / `JAMSESH_OBJECT_STORAGE_PATH_STYLE` (bool: "true"/"false")
- `ObjectStorageSyncQueueSize` / `JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE` (default 256)

Validation:
- `DeployMode=clustered` → `ObjectStorageURL` required (clear error on missing)
- `ObjectStorageSyncQueueSize <= 0` → rejected

Existing tests that used `JAMSESH_DEPLOY_MODE=clustered` without setting
`JAMSESH_OBJECT_STORAGE_URL` were updated to include the object-storage URL.

### main.go wiring (`cmd/portal/main.go`)

Object-storage wiring inserted after `storageSvc` and `eventLog` are constructed
(prerequisite for `Syncer.Storage`). In clustered mode: constructs Backend via
`objectstore.New`, wraps it in `ManifestStore` and `Syncer`. In single-instance
mode: `objSyncer` is nil. The postreceive `Emitter` receives `objSyncer` (nil
or non-nil); the Emitter already handles nil as a no-op (ships per pipeline story).

### Documentation updates

- `docs/SELF_HOST.md` §14: updated preview status framing (hydration-handoff is
  the last gap; routing + leases + durability are shipped). Added "Object storage
  (durability)" subsection with env-var reference table, per-provider deploy
  examples (AWS S3 / R2 / MinIO / GCS / Azure), IAM permissions, and cost model.
  Limitations section updated to list only hydration-handoff.
- `docs/SPEC.md`: added dual-layer description to Deployment shape section.
- `docs/SECURITY.md`: added object-storage IAM row to self-host operator
  responsibilities.
- `docs/ARCHITECTURE.md`: replaced "object-storage sync in progress" prose with
  "Bare-repo dual-layer storage" section describing the working-cache +
  system-of-record model, RPO=0 contract, fencing token enforcement, and
  conditional-write linearizability. Updated horizontal-scaling overview blurb.
