---
id: epic-cloud-native-deploy-lease-fencing
kind: feature
stage: review
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

## Architectural choice

**Selected: a `lease.Manager` interface with two impls (Noop + Postgres)
selected at startup by `JAMSESH_DEPLOY_MODE`. Every lease holds a
dedicated `*sql.Conn` for the duration to satisfy PG session-scoped
advisory-lock semantics.**

Considered:
- *Per-request lease acquire/release* — would re-acquire the lock on
  every request, hammering PG. Rejected.
- *Process-level mega-lock* — one lock per pod covering all owned
  sessions. Coarser, simpler, but kills the per-session sharding model.
- **Per-session lease with dedicated PG conn** — matches the
  pull-with-soft-coordinator epic design; one conn per active session
  is acceptable load (pgxpool sized for it via the pool-config story).

## Implementation Units

### Unit 1: Interface + NoopManager (single-instance shim)

**Files**:
- new: `internal/portal/lease/lease.go` — interface + sentinel errors
- new: `internal/portal/lease/noop.go` — `NoopManager` impl
- new: `internal/portal/lease/lease_test.go`

**Story**: `epic-cloud-native-deploy-lease-fencing-interface-and-noop`

```go
// internal/portal/lease/lease.go
package lease

import (
    "context"
    "errors"
)

// ErrAlreadyHeld is returned by Manager.Acquire when another pod (PG
// session) currently holds the lease for the requested session_id.
var ErrAlreadyHeld = errors.New("lease: session lease already held by another pod")

// Manager creates leases for portal sessions. The Postgres impl uses
// advisory locks to enforce mutual exclusion across pods; the Noop impl
// is a single-instance shim that always succeeds.
type Manager interface {
    // Acquire attempts a non-blocking lease acquisition for sessionID.
    // Returns ErrAlreadyHeld immediately if another pod holds the lock.
    // The returned Handle owns a dedicated PG connection (Postgres impl)
    // for the duration of the lease; Release MUST be called to free it.
    Acquire(ctx context.Context, sessionID string) (Handle, error)
}

// Handle is an active lease. Consumers (object-storage-sync,
// hydration-handoff) inspect FencingToken on every guarded operation
// and monitor Lost() to abort serving when the lease is gone.
type Handle interface {
    SessionID() string
    FencingToken() int64
    // Lost returns a channel that closes when the lease is lost
    // (PG session dies, heartbeat fails). Consumers should select on
    // this alongside their request contexts.
    Lost() <-chan struct{}
    // Release relinquishes the lease and frees the underlying PG conn.
    // Idempotent; safe to call after Lost() fires.
    Release() error
}
```

```go
// internal/portal/lease/noop.go
package lease

// NoopManager is the single-instance compatibility shim. Acquire never
// blocks and never returns ErrAlreadyHeld; the returned Handle's
// FencingToken is always 0 and Lost() never closes (until Release).
type NoopManager struct{}

func (NoopManager) Acquire(ctx context.Context, sessionID string) (Handle, error)
```

**Implementation Notes**:
- The Noop handle holds a `closed chan struct{}` that fires only on
  Release. Keeps the consumer-side `select` shape identical in both
  modes.
- Noop's `FencingToken() int64` returns 0. Consumers that fence on
  monotonic tokens (object-storage-sync) treat 0 as "no fencing
  required" and skip the conditional-write step in single-instance
  mode.

**Acceptance Criteria**:
- [ ] `Manager` and `Handle` interfaces compile and match the spec
- [ ] `NoopManager.Acquire` returns a Handle whose `FencingToken()==0`
- [ ] `Handle.Lost()` returns a channel that doesn't fire until
  `Release()`
- [ ] `Release()` is idempotent
- [ ] Unit tests cover Noop behavior + interface contract

### Unit 2: Schema migration + sqlc queries

**Files**:
- new: `internal/db/migrations/sqlite/N_leases.sql` and `..._down.sql`
- new: `internal/db/migrations/postgres/N_leases.sql` and `..._down.sql`
- new: `db/queries/leases.sql` — sqlc queries
- regen: `internal/db/sqlitestore/leases.sql.go` (sqlc generate)
- regen: `internal/db/pgstore/leases.sql.go`

**Story**: `epic-cloud-native-deploy-lease-fencing-schema`

Postgres schema:

```sql
CREATE SEQUENCE jamsesh_lease_fencing_tokens AS bigint;

CREATE TABLE leases (
    session_id     text        PRIMARY KEY,  -- one row per session
    pod_id         text        NOT NULL,
    fencing_token  bigint      NOT NULL,
    acquired_at    timestamptz NOT NULL DEFAULT now(),
    released_at    timestamptz,
    heartbeat_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX leases_released_at_idx ON leases(released_at)
    WHERE released_at IS NOT NULL;  -- supports retention cleanup
```

SQLite mirror (single-instance: the table is created but only ever has
zero rows because NoopManager doesn't touch it):

```sql
CREATE TABLE leases (
    session_id     TEXT PRIMARY KEY,
    pod_id         TEXT NOT NULL,
    fencing_token  INTEGER NOT NULL,
    acquired_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    released_at    TEXT,
    heartbeat_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX leases_released_at_idx ON leases(released_at)
    WHERE released_at IS NOT NULL;
```

Queries (`db/queries/leases.sql`):
- `IssueLeaseFencingToken` (PG only): `SELECT nextval('jamsesh_lease_fencing_tokens')`
- `InsertLease`: upsert by session_id; returns row
- `MarkLeaseReleased`: update released_at = now() WHERE session_id = ?
- `UpdateLeaseHeartbeat`: update heartbeat_at = now() WHERE session_id = ?
- `DeleteReleasedLeasesOlderThan`: for retention (PG only)

**Implementation Notes**:
- SQLite doesn't have sequences. Use `INTEGER PRIMARY KEY AUTOINCREMENT`
  semantics for fencing tokens IF SQLite-clustered ever happens; for
  now SQLite leases table is structural-only (NoopManager doesn't
  populate it).
- The PG sequence increments on EVERY call, even for failed-acquire
  attempts. That's fine — tokens being monotonic and globally ordered
  is the only invariant we need; gaps are acceptable.
- `leases.session_id` PK means re-acquire updates rather than inserts;
  but the advisory lock prevents two pods from both upserting the same
  row simultaneously, so the conflict is theoretical.

**Acceptance Criteria**:
- [ ] PG migration creates table, sequence, index
- [ ] SQLite migration creates table + index (structural; not used at
  runtime)
- [ ] sqlc generates Go code for the 5 queries (both dialects)
- [ ] `MigrateUp` is idempotent for both dialects
- [ ] Migration applies cleanly in fresh + existing-schema cases

### Unit 3: Postgres lease manager

**Files**:
- new: `internal/portal/lease/postgres.go` — `pgManager` + `pgHandle`
- new: `internal/portal/lease/postgres_test.go` — gated on
  `JAMSESH_TEST_PG_DSN`

**Story**: `epic-cloud-native-deploy-lease-fencing-postgres`

```go
// internal/portal/lease/postgres.go
package lease

// PostgresManager is the production lease implementation. Each
// successful Acquire holds a dedicated *sql.Conn for the lease's
// lifetime so the PG advisory lock (session-scoped) stays attributed
// to the owning pod.
type PostgresManager struct {
    DB         *sql.DB        // pgxpool-backed *sql.DB
    Store      store.Store    // for InsertLease / IssueLeaseFencingToken
    PodID      string         // identifies this pod in the leases table
    HeartbeatInterval time.Duration // default 10s
}

// Acquire follows this sequence:
//   1. Check out a dedicated *sql.Conn from DB
//   2. SELECT pg_try_advisory_lock(hashtext(sessionID))
//      - false → release conn, return ErrAlreadyHeld
//      - true  → proceed
//   3. SELECT nextval('jamsesh_lease_fencing_tokens') for the token
//   4. INSERT/UPDATE leases row with podID, token, acquired_at
//   5. Spawn heartbeat goroutine (pings the conn every HeartbeatInterval;
//      on failure closes Handle.Lost())
//   6. Return Handle
func (m *PostgresManager) Acquire(ctx context.Context, sessionID string) (Handle, error)
```

**Implementation Notes**:
- The dedicated `*sql.Conn` is critical — without it, the lock acquire
  and release could land on different PG sessions (same problem we
  identified during db-pool-and-lock story review). Use `db.Conn(ctx)`
  to dedicate.
- Heartbeat: `conn.PingContext(ctx)` every 10s. Failure (any error)
  closes `Lost()`. Background goroutine, lives until Release.
- Release order: stop heartbeat → `SELECT pg_advisory_unlock(...)` →
  `UPDATE leases SET released_at = now() WHERE session_id = ?` → close
  conn (returns it to pool).
- Conn lifetime: when the lease is held, this pod holds 1 PG conn per
  active session. With pgxpool MaxConns=25 (default), max ~24
  concurrent leases per pod (one conn reserved for normal query
  traffic). Document this in SELF_HOST when this lands.
- Collision detection: after acquiring the advisory lock, SELECT the
  leases row by session_id; if its pod_id matches a different pod and
  released_at is NULL, that's a hashtext collision — log a warning,
  release the lock, return ErrAlreadyHeld. (Extremely rare; documented
  defensive check.)

**Acceptance Criteria**:
- [ ] Acquire succeeds with valid sessionID; returns Handle with non-zero
  fencing token
- [ ] Second Acquire from same `*sql.DB` (different PG session via
  parallel call) returns ErrAlreadyHeld
- [ ] Handle.Lost() closes when the underlying PG conn drops (test by
  killing the conn from another connection: `pg_terminate_backend`)
- [ ] Release frees the conn and marks released_at
- [ ] Release after Lost() fires is idempotent
- [ ] Fencing tokens are monotonically increasing across multiple
  Acquire calls (no gaps required; just monotonic)
- [ ] Heartbeat keeps the lease alive across natural idle periods
- [ ] Integration test gated on `JAMSESH_TEST_PG_DSN`

### Unit 4: Factory selection + retention + metrics

**Files**:
- new: `internal/portal/lease/factory.go` — `New(cfg, store) Manager`
- new: `internal/portal/lease/retention.go` — periodic cleanup goroutine
- edit: `internal/portal/metrics/metrics.go` — add lease metric handles
- edit: `internal/portal/config/config.go` — add lease-related config
- edit: `cmd/portal/main.go` — wire up Manager + retention goroutine

**Story**: `epic-cloud-native-deploy-lease-fencing-factory-and-retention`

```go
// internal/portal/lease/factory.go
package lease

// New returns the Manager appropriate for the configured deploy mode.
// "single" → NoopManager; "clustered" → PostgresManager backed by db.
func New(deployMode string, db *sql.DB, store store.Store, podID string) Manager {
    if deployMode != "clustered" {
        return NoopManager{}
    }
    return &PostgresManager{DB: db, Store: store, PodID: podID, HeartbeatInterval: 10 * time.Second}
}
```

Retention:

```go
// internal/portal/lease/retention.go
package lease

// RunRetention periodically deletes released leases older than
// retentionAfter. No-op for NoopManager (no rows to delete). Blocks
// until ctx is cancelled.
func RunRetention(ctx context.Context, store store.Store, interval, retentionAfter time.Duration)
```

Metrics (added to `internal/portal/metrics/metrics.go`):
- `jamsesh_lease_acquires_total{result}` — result in {ok, conflict, error}
- `jamsesh_lease_holds_currently` — gauge
- `jamsesh_lease_hold_duration_seconds` — histogram, observed at Release
- `jamsesh_lease_lost_total` — counter
- `jamsesh_lease_fencing_tokens_issued_total` — counter (same as
  successful acquires; useful as a sanity-check)

Config additions:
- `JAMSESH_DEPLOY_MODE` (already pinned by epic; the lease factory reads it)
- `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S` (default 10)
- `JAMSESH_LEASE_RETENTION_DAYS` (default 30)
- `JAMSESH_LEASE_RETENTION_INTERVAL_HOURS` (default 1)

**Acceptance Criteria**:
- [ ] `lease.New("single", ...)` returns `NoopManager`
- [ ] `lease.New("clustered", ...)` returns `*PostgresManager`
- [ ] `RunRetention` deletes rows where `released_at < NOW() - interval`
- [ ] Metrics emit on Acquire (ok/conflict/error) and Release
- [ ] `cmd/portal/main.go` wires up Manager + starts retention goroutine
  in clustered mode only
- [ ] Single-instance mode: no PG queries against `leases` table

## Implementation Order

Wave 1 (parallel): Unit 1 (interface+noop), Unit 2 (schema+queries)
Wave 2: Unit 3 (postgres manager) — depends on 1+2
Wave 3: Unit 4 (factory+retention+metrics) — depends on 3

## Testing

| Unit | Type | Surfaces |
|---|---|---|
| 1 interface+noop | unit | interface compliance, Noop's never-blocks/never-loses contract |
| 2 schema | unit (sqlite); integration (pg) | migration idempotency, sequence increments |
| 3 postgres | integration (gated on JAMSESH_TEST_PG_DSN) | acquire/conflict/lost/release; heartbeat; backend-terminate-triggers-Lost |
| 4 factory+retention | unit (factory); integration (retention pg) | deploy-mode selection, retention cleanup |

## Risks

- **Connection-per-lease scaling**: a pod with N active session leases
  uses N+1 PG conns. With pgxpool MaxConns=25, that caps at ~24
  concurrent leases per pod. For a cluster serving thousands of
  sessions, requires either larger pool or more pods. Document the
  math in SELF_HOST.
- **Hashtext collisions**: `hashtext(session_id)` is a 32-bit hash;
  birthday-bound collision probability is ~2^16 sessions. The defensive
  check in Acquire (compare session_id in leases row) catches it but
  emits a false ErrAlreadyHeld. At session-cardinality scales this is
  vanishingly rare; documented.
- **Heartbeat false negatives**: a transient network blip on the
  dedicated conn fires Lost() spuriously. The pod aborts serving and
  the router re-routes. Cost is one 503 + a re-acquire on the next
  request — acceptable. The 10s heartbeat interval is a tunable.
- **Released lease rows accumulate**: bounded by `RunRetention` running
  on the configured interval. If retention goroutine dies, the table
  grows unbounded. Metric `jamsesh_lease_acquires_total` is a proxy for
  unbounded growth — alert on it if needed.

## Foundation-doc impact

- `docs/SPEC.md` — clustered-mode hard constraint about Postgres
  required (already documented in operational-polish; reaffirm).
- `docs/ARCHITECTURE.md` — request-lifecycle gains the lease check
  in clustered mode.
- `docs/SELF_HOST.md` — connection-per-lease math + retention env vars.

## Children complete (2026-05-17)

All 4 child stories landed and reviewed:

| Story | Verdict | Notes |
|---|---|---|
| interface-and-noop | Approve | Manager + Handle interfaces + NoopManager; sync.Once for idempotent Release |
| schema | Approve with comments | leases table + sequence + sqlc queries. sqlc hand-written (no sqlc in env); backlog `lease-fencing-schema-verify-sqlc-regen` |
| postgres | Approve | PostgresManager with dedicated `*sql.Conn` + heartbeat; 8 integration tests gated on JAMSESH_TEST_PG_DSN |
| factory-and-retention | review (pending) | factory + retention goroutine + 5 metric handles + config validation; db.Open signature changed to return *sql.DB |

Verification: `go build ./...` clean; `go test ./...` green across all packages.

Feature advanced `implementing → review`. db.Open signature change touched 25+ test files; sibling story (factory-and-retention) handled the cascade.
