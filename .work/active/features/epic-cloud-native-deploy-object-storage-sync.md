---
id: epic-cloud-native-deploy-object-storage-sync
kind: feature
stage: drafting
tags: [portal]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-lease-fencing]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Object-Storage Sync

## Brief

In clustered mode, makes object storage (GCS / S3 / Azure Blob / any
S3-compatible) the system of record for bare repos. Local disk becomes
a working cache. Every write to the lease holder's local bare repo is
mirrored to object storage continuously, with fencing-token gating so
stale writes from a former lease holder are rejected.

The continuous-sync model exploits git's content-addressed object
database: `objects/xx/yyyy…` files are immutable once written, so we
can upload them as they appear. The mutable bits (`refs/*`,
`packed-refs`, pack-file rewrites from `git gc`) use conditional writes
(generation match / `If-Match` ETag) to maintain a single linearizable
history per session.

Single-instance mode skips this entirely — local disk remains system
of record.

## Scope

In:
- `internal/portal/storage/objectstore/` — implementation of the
  existing `storage.Service` interface that wraps the local-FS Service
  and emits sync events on every write. Implementations for at least
  S3 (covers AWS / R2 / B2 / MinIO via API compatibility), GCS, and
  Azure Blob. Selected by URL scheme in `JAMSESH_OBJECT_STORAGE_URL`.
- Sync pipeline:
  - Hook into the existing `post-receive` boundary in
    `internal/portal/postreceive/` — enumerate new objects, refs,
    packs in the push.
  - Per-object: upload to object storage with fencing token in object
    metadata. Object DB files are content-addressed → upload is
    idempotent; no conditional write needed.
  - Per-ref: conditional write keyed on previous ref value (S3
    `If-Match` ETag, GCS `ifGenerationMatch`, Azure
    `If-Match`). On precondition failure, abort the entire push —
    means our lease has been lost or fenced. Fail-stop.
  - Per-pack rewrite (`git gc`): upload new pack + idx, swap pointer
    in a manifest file (conditional write), enqueue lazy deletion of
    old packs.
- Pack manifest convention: a single object per session
  (e.g. `sessions/<id>/manifest.json`) listing current pack files,
  current ref map, current packed-refs content. Hydration uses this to
  know what to download.
- Bounded async upload queue per session with backpressure: if uploads
  fall behind a threshold, reject new pushes with 503 (push-per-commit
  is acceptable; falling-far-behind is not).
- Fencing token validation: every upload includes the lease's fencing
  token in object metadata. Object storage doesn't enforce this for us
  (no native fencing support); instead, before any conditional write
  we read the manifest's "last-known fencing token" and verify ours is
  ≥. If a stale lease holder writes between our read and write, the
  conditional write on the manifest itself catches the race.
- Metrics: upload rate, upload lag (time from disk-write to
  object-store-acked), upload failures by category, fencing rejections.
- Operator config: `JAMSESH_OBJECT_STORAGE_URL`,
  `JAMSESH_OBJECT_STORAGE_REGION`, credentials per provider
  conventions (env vars or workload identity).

Out:
- Hydration / download path — that's `hydration-handoff`.
- Multi-region replication. Single-bucket-single-region in v1; cross-
  region replication is the object store's job to configure (S3 CRR,
  GCS dual-region, etc.) and a future epic to actually exercise.
- Replacing local FS entirely. Local FS remains the working surface;
  object storage is mirror + truth.
- Archive path. The existing `storage.Service.ArchiveSession` already
  has a clean boundary; archival in clustered mode can later route
  to a cheaper storage class (Glacier / Coldline / Archive), but
  initial implementation just deletes from hot object storage.

## Strategic decisions

- **Object storage is mirror, not async backup.** A push doesn't ack
  to the client until the corresponding objects + ref updates are
  durable in object storage. RPO = 0 for acknowledged pushes. This is
  the only safe contract given the client treats acked pushes as
  durable.
- **S3-compatible API as the lowest common denominator.** Native SDK
  per provider gives better auth integration (workload identity,
  managed service auth), but S3-compat covers MinIO / R2 / B2 /
  self-hosted Ceph. Ship native SDKs for the big three (AWS, GCS,
  Azure) plus a generic S3-compatible mode for everyone else.
- **Pack manifest as the read-side index.** Object storage listings
  are eventually consistent on some providers; a manifest object
  written with conditional writes is the linearizable source.
- **Subprocess git stays on local disk.** We don't try to make
  `git-receive-pack` write directly to object storage. The lease
  holder runs subprocess git against local disk, then syncs. This
  preserves performance and avoids a year of pure-Go smart-HTTP
  work. Trade-off: a pod that's the lease holder MUST have functional
  local disk for the duration of the lease.

## Foundation-doc impact

- `docs/SPEC.md` — `storage.Service` semantics gain an "object storage
  optional, system of record when configured" sentence when this lands.
- `docs/ARCHITECTURE.md` — bare-repo storage section gains the dual-
  layer (working cache + object-store truth) description.
- `docs/SECURITY.md` — object storage adds a new persistence boundary
  with its own auth surface (IAM role, service account, access key).
  Add to the operator-responsibilities table.

## Notes for design

The `storage.Service` interface (`internal/portal/storage/service.go`)
returns paths today. This feature doesn't need to change that — the
local-disk path stays valid for subprocess git. Instead, this feature
*wraps* the existing service: writes happen to local disk first
(unchanged), then the wrapper enqueues sync to object storage. The
post-receive hook is the natural mirror trigger.

The hardest sub-problem is `git gc` mid-session. Default git config
runs gc opportunistically after pushes. Two options: (a) disable
opportunistic gc per session repo and run scheduled gc only when no
pushes are in-flight, (b) handle pack-rewrite events as first-class
sync operations. Resolve in design.

Cost model from earlier analysis: ~$0.05/active-session/day in object
storage API calls at heavy use; storage cost negligible. Worth
documenting in SELF_HOST so operators sizing their bills don't get
surprised.
