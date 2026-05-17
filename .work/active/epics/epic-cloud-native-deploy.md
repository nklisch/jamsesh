---
id: epic-cloud-native-deploy
kind: epic
stage: drafting
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

## Strategic decisions

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

## Phased delivery

**Phase 1 — single-instance polish (ship independently).**
- `epic-cloud-native-deploy-operational-polish`

**Phase 2 — clustered-mode primitives (opt-in).**
- `epic-cloud-native-deploy-routing-layer`
- `epic-cloud-native-deploy-lease-fencing`
- `epic-cloud-native-deploy-object-storage-sync`
- `epic-cloud-native-deploy-hydration-handoff`

Dependency graph:

```
operational-polish
├── routing-layer
└── lease-fencing
    └── object-storage-sync
        └── hydration-handoff   (also depends on routing-layer)
```

Phase 1 has standalone value and a clear ship boundary. Phase 2 features
build on each other and ship together as the clustered-mode capability.

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
