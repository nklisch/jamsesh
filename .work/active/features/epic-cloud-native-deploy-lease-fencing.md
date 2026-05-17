---
id: epic-cloud-native-deploy-lease-fencing
kind: feature
stage: drafting
tags: [portal]
parent: epic-cloud-native-deploy
depends_on: [epic-cloud-native-deploy-operational-polish]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Cloud-Native Deploy — Lease + Fencing

## Epic context

- Parent epic: `epic-cloud-native-deploy`
- Position in epic: phase-2 coordination primitive. Independent of
  routing-layer (parallel design / implementation possible). The
  primitive consumed by object-storage-sync (fencing token gates
  every object-storage write) and hydration-handoff (lease lifecycle
  triggers hydration on acquire and eviction on loss).

## Foundation references

- `docs/SPEC.md` — "Multi-tenant by design / Self-host-capable" hard
  constraints (the SQLite path needs a no-op shim so single-instance
  doesn't pay the coordination cost).
- `docs/ARCHITECTURE.md` — "Data layer (multi-tenancy)" (this feature
  adds a new `leases` table to the schema following the org_id-in-WHERE
  convention).
- `internal/db/store/` and `internal/db/migrations/` — sqlc query
  patterns and migration conventions this feature follows.
- `internal/portal/finalize/lock_acquire.go` — pre-existing Postgres-
  coordinator pattern in the codebase (different purpose, but
  validates the approach).

## Brief

The distributed coordination primitive that lets multiple portal pods
serve different sessions safely without ever serving the same session
concurrently. Adds:

- A per-session lease, acquired via Postgres advisory lock keyed by
  session_id. Whoever holds the lock owns the session.
- A monotonically-increasing fencing token (Postgres sequence) attached
  to every lease grant. Used downstream by
  `epic-cloud-native-deploy-object-storage-sync` to reject stale writes
  from a previous lease holder if a network blip causes split-brain.
- Lease lifecycle: acquire → heartbeat → release on idle / shutdown /
  loss.
- Lease loss detection: if the Postgres session dies (network blip,
  connection reset), the pod's lease lock is automatically released —
  the pod must stop serving the session immediately and return 503
  until it re-acquires.

This feature provides the primitive only. The two consumers
(`object-storage-sync` and `hydration-handoff`) build on top of it.

Activated only in clustered mode. Single-instance mode skips the lease
acquisition step entirely (the single pod implicitly owns every session).

## Scope

In:
- `internal/portal/lease/` package providing a `Manager` interface:
  - `Acquire(ctx, sessionID) (LeaseHandle, error)` — non-blocking try;
    returns `ErrAlreadyHeld` if another pod has it.
  - `LeaseHandle.FencingToken() int64`
  - `LeaseHandle.Release() error`
  - Heartbeat goroutine internal to handle.
- Postgres schema: a `leases` table for fencing-token issuance and
  audit (`session_id`, `pod_id`, `token`, `acquired_at`, `released_at`),
  plus the `pg_advisory_lock` invocation that does the actual mutex.
- Integration into the request lifecycle: portal pods acquire the lease
  on first request for a session, hold it through the idle window
  (configurable, default 5 minutes after last request), release on
  shutdown or idle timeout.
- Lease-loss handler: on advisory-lock loss (detected via PG keepalive),
  immediately stop serving the session, evict local cache (when
  `epic-cloud-native-deploy-object-storage-sync` lands), return 503 to
  in-flight requests so the router retries.
- Metrics: lease acquisition rate, lease hold duration distribution,
  lease-loss events, fencing token issuance.
- Single-instance compatibility shim: a `NoopManager` that returns a
  handle with token=0 and never blocks. Selected when
  `JAMSESH_OBJECT_STORAGE_URL` is unset.

Out:
- Object-storage writes that use the fencing token — that's
  `object-storage-sync`.
- Cache eviction logic — that's `hydration-handoff`.
- The router's "which pod has the lease?" hint — router uses
  consistent hashing in v1; a hint table is future work.

## Design decisions

Inherited from epic (per-session lease boundary; pull-with-soft-
coordinator acquisition; fencing tokens non-negotiable; Postgres
advisory locks over a dedicated coordinator). Feature-local:

- **Advisory-lock key formula: `pg_try_advisory_lock(hashtext($session_id))`.**
  PG advisory locks take int64 keys; `hashtext` gives us deterministic
  mapping from session_id strings. Collision risk on hashtext is
  negligible at session-cardinality scales, but feature design should
  document detection (compare session_id stored in our `leases` table
  before assuming we own a lock).
- **Fencing tokens come from a Postgres sequence, not a clock.** Clocks
  drift; sequences are monotonic and globally ordered. Cost is one
  extra round-trip on lease acquisition.
- **Lease-loss is fail-stop, not fail-over.** A pod that loses its
  lease must stop serving — it does not try to re-acquire mid-flight.
  The router observes 503 and re-routes; the new pod (which may be the
  same pod a moment later) acquires fresh. Avoids ABA-style bugs.
- **No-op shim for single-instance.** A `NoopManager` returns a handle
  with token=0 and never blocks. Selected when
  `JAMSESH_DEPLOY_MODE` is `single` (default). Keeps the call-site
  pattern identical across both modes — feature lands as a clean
  interface insertion, not a conditional.

## Foundation-doc impact

- `docs/SPEC.md` — clustered mode adds the lease/fencing constraint to
  hard constraints when this lands.
- `docs/ARCHITECTURE.md` — lease lifecycle becomes part of the request
  flow description in clustered mode.

## Notes for design

Postgres `pg_try_advisory_lock` is session-scoped (the PG session,
not jamsesh session). The portal pod must hold a dedicated PG
connection for the duration of any lease it holds. Pool sizing
implications: a pod that leases N sessions concurrently needs N+1
connections (one per lease + one for normal query traffic). Document
this in the SELF_HOST guidance.

The fencing-token table is append-only and could grow indefinitely; add
a retention story (drop rows older than 30 days, say) in this feature.
