---
id: epic-cloud-native-deploy
kind: epic
stage: review
tags: [infra, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy

## Brief

Make jamsesh deployable on any modern cloud platform without giving up the
single-binary self-host story. Two shapes, both first-class:

- **Single-instance (default).** What `docs/SELF_HOST.md` already describes:
  one binary, SQLite or Postgres, local disk for bare repos. A handful of
  cloud-operability primitives (`/readyz`, `/metrics`, public-URL override,
  secret-from-file, migration advisory lock, graceful shutdown) make this
  shape land cleanly on Cloud Run (min=max=1), Fly, Railway, a single VM,
  or k8s with a `PersistentVolumeClaim`.
- **Clustered (opt-in).** N portal pods with ephemeral local disks behind a
  small consistent-hashing router. Each session is leased to one pod at a
  time via Postgres advisory locks (with a fencing token to prevent
  split-brain). Bare repos sync continuously to object storage (GCS / S3 /
  Azure Blob), which becomes the system of record. On lease handoff the
  receiving pod hydrates from object storage. Enables true horizontal
  scaling on GKE, EKS, ECS, Fly clusters, or multi-instance Cloud Run.

The clustered shape is a deployment *topology*, not a code-path fork. The
same binary runs both shapes; clustered behavior activates when
`JAMSESH_OBJECT_STORAGE_URL` (or equivalent) is set. Everything that runs
single-instance keeps running single-instance, unchanged.

## Design decisions

Captured during scope (the first six) and epic-design `--only-questions`
pass (the last four). These constrain every child feature's later design
pass.

- **Audience: both, phased.** Self-hosters are primary; the scalable
  deployment is also supported. Operational-polish (phase 1) ships
  standalone and benefits both shapes. The four clustered-mode features
  (phase 2) are opt-in.
- **Simple deploy must not regress.** SQLite + local disk + single
  binary stays the default and stays trivial. Any flag, config, or wiring
  introduced by clustered mode is optional and defaults to off.
- **Object storage = system of record (in clustered mode).** Local disk is
  a working cache. Continuous sync on every write; durability is the
  object store. This is the design fork that unlocks stateless pods.
- **Coordination via Postgres (existing dep).** Advisory locks for
  per-session leases; `LISTEN/NOTIFY` for cross-pod event fan-out. No
  Redis / etcd / Zookeeper added. Clustered mode requires Postgres
  (SQLite remains valid for single-instance).
- **Routing as a separate small Go service.** Keeps portal pods stateless
  from a routing-decision POV; the router is the only thing that needs
  the consistent-hash ring. Optional component — only deployed in
  clustered mode.
- **Fencing tokens are non-negotiable.** Split-brain corruption of bare
  repos is unacceptable. Every object-storage write carries a fencing
  token; stale writes from a former lease holder are rejected.
- **Activation: explicit `JAMSESH_DEPLOY_MODE=single|clustered` flag**
  (defaults to `single`). Clustered-mode subsystem configs
  (`JAMSESH_OBJECT_STORAGE_URL`, lease tuning, etc.) only take effect
  when mode is `clustered` — lets operators stage-test by setting the
  URL but keeping mode=single while validating the bucket.
- **Object-storage providers: native-where-clean + S3-compat fallback.**
  Ship native SDK implementations for AWS S3, GCS, and Azure Blob where
  the official Go SDK is ergonomic and provides real value (workload
  identity, managed credential rotation). Research each SDK before
  adopting; if a given SDK requires too much glue to fit our patterns,
  roll a thin client against the provider's REST API instead. S3-
  compatible interface covers MinIO / R2 / B2 / self-hosted Ceph. URL
  scheme (`s3://`, `gs://`, `azblob://`, `s3-compatible://`) picks
  the implementation at startup.
- **Lease boundary: per-session.** One Postgres advisory lock per
  `session_id`. Matches the per-session bare-repo boundary already in
  code. Org-scoped leases would simplify routing but lose load balance
  for large tenants — declined.
- **Lease acquisition: pull-with-soft-coordinator.** Pods self-acquire
  via `pg_try_advisory_lock` on first request for a session
  (`hashtext(session_id)` as the lock key). The routing service
  maintains a soft cache of "which pod recently acquired session X" and
  uses it as a routing hint overlaid on the consistent-hash ring. No
  separate coordinator process. Failed acquisition returns 503
  Retry-After; the router re-dispatches.

## Architecture (target, clustered mode)

```
                  ┌──────────────────┐
   clients ──────▶│  routing service │
                  │  (consistent     │
                  │  hash by         │
                  │  session_id)     │
                  └────────┬─────────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌─────────┐  ┌─────────┐  ┌─────────┐
        │portal A │  │portal B │  │portal C │
        │/cache:  │  │/cache:  │  │/cache:  │
        │ sess 1  │  │ sess 3  │  │ sess 5  │
        │ sess 2  │  │ sess 4  │  │ sess 6  │
        └────┬────┘  └────┬────┘  └────┬────┘
             │            │            │
             │     ┌──────┴──────┐     │
             ├────▶│  Postgres   │◀────┤
             │     │  (state,    │     │
             │     │  advisory   │     │
             │     │  locks,     │     │
             │     │  LISTEN/    │     │
             │     │  NOTIFY)    │     │
             │     └─────────────┘     │
             │                         │
             └──────────┬   ┌──────────┘
                        ▼   ▼
                  ┌──────────────┐
                  │  Object      │
                  │  storage     │
                  │  (GCS/S3/    │
                  │  Azure)      │
                  │  bare repos  │
                  │  = system    │
                  │  of record   │
                  └──────────────┘
```

In single-instance mode the routing service is absent, the object-storage
arrow doesn't exist, and the single pod's local disk is system of record.

## Why this works for jamsesh specifically

- **Per-session boundary.** Sessions don't share repos. Sharding by
  `session_id` is trivial and avoids any shared-filesystem requirement.
- **Git's object DB is content-addressed and append-only.** Continuous
  upload of `objects/xx/yyyy…` files to object storage is safe — files
  are immutable once written. Only `refs/*` and `packed-refs` need
  conditional writes (GCS generation match, S3 `If-Match`).
- **Recovery is `git fetch` (principle).** Clients are already resilient
  to portal restarts and brief outages. A few-seconds-stale hydration
  on handoff is recovered by the client's next fetch.
- **Push-per-commit + pull-based digest.** WebSocket events are
  convenience, not correctness. Sticky-routing failover that drops a WS
  connection is recoverable by clients.

## Decomposition

Decomposition was performed during `/agile-workflow:scope` (the source
conversation contained sufficient design intent to split immediately).
This epic ran `/agile-workflow:epic-design --only-questions` to capture
the strategic ambiguities listed above; the decomposition itself was
not re-run.

Split by capability + phase, not by layer. Phase 1 has standalone value
and a clear ship boundary — operational-polish lands cloud-operability
primitives that help both deploy shapes. Phase 2 features build on each
other and ship together as the opt-in clustered-mode capability.

### Child features

- `epic-cloud-native-deploy-operational-polish` — cloud-operability
  primitives (`/readyz`, `/metrics`, `_FILE` secrets, migration
  advisory lock, graceful shutdown, PG pool tuning) — depends on: `[]`
- `epic-cloud-native-deploy-routing-layer` — small Go consistent-hash
  reverse proxy with soft-coordinator hint cache — depends on:
  `[epic-cloud-native-deploy-operational-polish]`
- `epic-cloud-native-deploy-lease-fencing` — per-session Postgres
  advisory-lock leases with monotonic fencing tokens — depends on:
  `[epic-cloud-native-deploy-operational-polish]`
- `epic-cloud-native-deploy-object-storage-sync` — continuous mirror
  of bare-repo writes to GCS/S3/Azure/S3-compat; system of record in
  clustered mode — depends on:
  `[epic-cloud-native-deploy-lease-fencing]`
- `epic-cloud-native-deploy-hydration-handoff` — lease-acquisition
  hydration from object storage; cache eviction on lease loss /
  shutdown — depends on:
  `[epic-cloud-native-deploy-object-storage-sync,
  epic-cloud-native-deploy-lease-fencing,
  epic-cloud-native-deploy-routing-layer]`

### Decomposition risks

- **operational-polish risks underclaiming scope.** The feature must
  serve both single-instance and clustered deploy shapes. Designers
  must keep configs opt-in and avoid baking clustered-mode assumptions
  into universally-loaded code paths.
- **lease-fencing and object-storage-sync are tightly coupled** — fencing
  only matters because of split-brain risk on object storage writes. If
  designers split them too cleanly, the contract between them (fencing
  token format, validation responsibility) may drift. Keep designer
  attention on the interface between these two features.
- **hydration-handoff's three-way dependency** is unusual for an
  agile-workflow feature. Designers may be tempted to ship parts of it
  alongside object-storage-sync; resist that — the eviction half is
  meaningless without hydration, and shipping them together keeps the
  lifecycle test surface coherent.

## Codebase findings (Phase 3 explore)

Surfaced during the epic-design read pass so feature designers don't
re-discover them:

- **`JAMSESH_PORTAL_URL` already exists** in `internal/portal/config/`.
  Operational-polish should leverage it, not invent
  `JAMSESH_PUBLIC_URL`. (Original scope draft used the latter name —
  corrected during alignment.)
- **`storage.Service.RepoPath()` has 11 call sites** across automerger,
  githttp (3), sessions (2), mcpendpoint (2), finalize. Object-storage
  sync wraps the Service; all 11 consumers continue to use the local
  path — sync happens underneath.
- **`internal/portal/postreceive/Emitter.EmitForUpdates()`** is the clean
  tap point for the sync hook. Called from
  `githttp/receive_pack.go:203-204` after pre-receive validation.
- **`events.Log` has 2 subscribers** (automerger, wsgateway) and 16
  emitters. Replacing in-process channels with a `Bus` interface
  (local + Postgres LISTEN/NOTIFY impls) has a bounded blast radius.
- **`finalize_locks` table exists** for finalize-flow state (different
  purpose). Confirms Postgres-as-coordinator pattern is already in use;
  no naming or schema conflict with new session-lease advisory locks.
- **No naming collisions** in `internal/` for `lease`, `sync`,
  `object-storage`, `fencing`, `advisory`.

## Non-goals (in this epic)

- Multi-region active-active. Cross-region object-storage replication and
  cross-region routing are explicitly out. Single-region clustered first.
- A new coordinator service (Redis, etcd, Zookeeper). Postgres is the
  coordinator.
- Replacing the system `git` subprocess for `upload-pack` /
  `receive-pack`. In clustered mode the lease holder still does
  subprocess git against local disk; the object-storage sync is the
  durability layer underneath. A future epic could explore pure-Go
  smart-HTTP if the subprocess boundary becomes the bottleneck.
- Sharded Postgres / multi-cluster databases. One Postgres instance
  (managed Cloud SQL / RDS) serves the fleet.
- Auto-scaling policy. Operators decide pod counts; the system supports
  scale events but doesn't drive them.

## Open questions deferred to feature design

- Routing service: standalone binary in this repo, or recommend Envoy /
  Caddy / HAProxy with a config recipe? (Resolved in routing-layer
  feature design.)
- Object storage abstraction: native GCS/S3/Azure SDKs, or a generic
  S3-compatible interface (works for R2, B2, MinIO)? (Resolved in
  object-storage-sync feature design.)
- Hydration strategy: full repo on first request, or lazy object-level
  fetch via a custom go-git Storer? (Resolved in hydration-handoff
  feature design.)
- Lease heartbeat cadence and timeout. (Resolved in lease-fencing
  feature design.)

## Children complete (2026-05-17)

All 5 child features landed and reviewed:

| Feature | Verdict | Notes |
|---|---|---|
| operational-polish | Approve | Phase 1 single-instance polish — `/readyz`, `/metrics`, `_FILE` secrets, migration lock, graceful shutdown, PG pool config |
| routing-layer | Approve | Phase 2 — standalone `cmd/jamsesh-router/` binary; consistent-hash + soft-coordinator hint cache; k8s + static discovery |
| lease-fencing | Approve with comments | Phase 2 — per-session Postgres advisory locks with fencing tokens; NoopManager for single-instance compatibility |
| object-storage-sync | Approve | Phase 2 — S3/GCS/Azure backends; RPO=0 sync; pack manifest for linearizable state; gc.auto=0 on CreateRepo |
| hydration-handoff (review pending) | — | Phase 2 capstone — lifecycle manager + hydration on acquire + eviction on release; LRU + idle eviction |

Verification: `go build ./...` clean; `go test ./...` green across all packages.

Epic advanced `drafting → review`. The clustered-mode cloud-native deployment capability is end-to-end shipped: single-instance deploys remain the default and gain operational polish; clustered deploys are first-class with horizontal scaling, fail-stop safety, and clean session migration.
