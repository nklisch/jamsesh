---
id: epic-cloud-native-deploy-lease-fencing-factory-and-retention
kind: story
stage: done
tags: [portal]
parent: epic-cloud-native-deploy-lease-fencing
depends_on: [epic-cloud-native-deploy-lease-fencing-postgres]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Lease+Fencing — Factory + retention + metrics

## Scope

Factory function `lease.New(deployMode, db, store, podID)` returning
NoopManager or PostgresManager. Background retention goroutine that
periodically deletes released-lease rows older than the configured
retention window. Prometheus metrics for acquire/release/lost/heartbeat
events. Wire-up in `cmd/portal/main.go`.

Implements **Unit 4** of `epic-cloud-native-deploy-lease-fencing`.

## Files

New:
- `internal/portal/lease/factory.go`
- `internal/portal/lease/retention.go`
- `internal/portal/lease/factory_test.go`
- `internal/portal/lease/retention_test.go`

Edit:
- `internal/portal/metrics/metrics.go` — add lease metric handles
- `internal/portal/config/config.go` — `JAMSESH_LEASE_*` env vars
- `cmd/portal/main.go` — wire up Manager + retention goroutine

## Config additions

- `JAMSESH_DEPLOY_MODE` — already pinned by epic; values `single` (default)
  or `clustered`
- `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S` — default 10
- `JAMSESH_LEASE_RETENTION_DAYS` — default 30
- `JAMSESH_LEASE_RETENTION_INTERVAL_HOURS` — default 1

## Metric handles (added to `internal/portal/metrics/metrics.go`)

- `jamsesh_lease_acquires_total{result}` — result in {ok, conflict, error}
- `jamsesh_lease_holds_currently` (gauge)
- `jamsesh_lease_hold_duration_seconds` (histogram; observed at Release)
- `jamsesh_lease_lost_total` (counter)
- `jamsesh_lease_fencing_tokens_issued_total` (counter)

## Acceptance criteria

- [ ] `lease.New("single", ...)` returns `NoopManager`
- [ ] `lease.New("clustered", ...)` returns `*PostgresManager` with
  populated fields
- [ ] `RunRetention` deletes rows where `released_at < NOW() - retention`
  on each tick
- [ ] Metrics emit on Acquire (ok/conflict/error), Release (hold duration),
  and Lost
- [ ] `cmd/portal/main.go` wires up Manager in clustered mode and starts
  retention goroutine
- [ ] Single-instance mode (default): no PG queries against `leases`
  table; retention goroutine is not started
- [ ] Lease config env vars validate via `config.validate()` (positive
  integers for intervals/days)

## Notes

- Retention goroutine should respect ctx cancellation for graceful
  shutdown.
- Wire-up in main.go: construct Manager AFTER db.Open returns AND after
  metrics Registry exists, since PostgresManager needs both.
- When `DEPLOY_MODE=clustered` AND `DB_DRIVER=sqlite`, fail at startup —
  clustered mode requires Postgres. Validate in `config.validate()`.
- Tests for retention can use the existing PG container pattern.

## Implementation notes

### Design choices

- `db.Open` signature changed from `(store.Store, error)` to
  `(store.Store, *sql.DB, error)`. The `*sql.DB` is needed by
  `PostgresManager` to call `db.Conn(ctx)` for dedicated connections.
  All call sites updated — the pattern is `s, _, err := db.Open(...)` in
  tests that don't need the `*sql.DB`.

- For Postgres, the returned `*sql.DB` is a `stdlib.OpenDBFromPool` bridge
  over the same `pgxpool.Pool` used by the adapter. Connections still come
  from the pool; the bridge gives `database/sql` semantics (`db.Conn`)
  on top of the pgx pool.

- `PostgresManager` gains an optional `Metrics *metrics.Registry` field.
  All metric helpers are nil-safe inline helpers on `*PostgresManager` —
  no interface changes, no allocation on the nil path.

- `pgHandle` gains `acquiredAt time.Time` and `mgr *PostgresManager` to
  support hold-duration observation at Release and LeaseLost emission in
  the heartbeat goroutine.

- `lease.New` takes `heartbeatInterval time.Duration` and `metricsReg
  *metrics.Registry` as explicit parameters (rather than reading from a
  config struct) to keep the factory free of a config dependency and stay
  consistent with the PostgresManager constructor pattern.

- `RunRetention` returns `error` (context.Canceled on normal exit) so the
  caller can log the stop reason. It logs at `slog.Debug` on each successful
  tick and `slog.Warn` on error.

- `cmd/portal/main.go` derives `podID` from `$HOSTNAME` (set by Kubernetes)
  with `os.Hostname()` as fallback — no new config field needed.

### Files changed

New:
- `internal/portal/lease/factory.go`
- `internal/portal/lease/factory_test.go`
- `internal/portal/lease/retention.go`
- `internal/portal/lease/retention_test.go`

Modified:
- `internal/portal/lease/postgres.go` — added `Metrics` field, nil-safe
  helpers, metrics emission in Acquire/Release/runHeartbeat
- `internal/portal/metrics/metrics.go` — 5 lease metric handles appended
- `internal/portal/config/config.go` — 4 new fields + validation rules
- `internal/portal/config/config_test.go` — 7 new test functions, updated
  clearEnv
- `internal/db/connect.go` — db.Open returns (store.Store, *sql.DB, error)
- `internal/db/connect_test.go` — updated for new signature
- All test files that called db.Open — updated to ignore the *sql.DB with _
- `cmd/portal/main.go` — lease manager wiring, retention goroutine

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `db.Open` signature changed from `(store.Store, error)` to `(store.Store, *sql.DB, error)` — needed by PostgresManager for `db.Conn(ctx)`. Touched 25+ test files. Reasonable trade-off; the alternative (exposing `*sql.DB` through the Store interface) would have leaked the dialect-specific connection type.

**Notes**: Factory + retention + metrics all clean. `lease.New(deployMode, db, store, podID, heartbeatInterval, metricsReg)` returns NoopManager (single mode) or PostgresManager (clustered). RunRetention is a ctx-aware goroutine that ticks on the configured interval; exits cleanly on cancellation; returns `context.Canceled` on normal stop.

Metrics emission added to PostgresManager via nil-safe helpers — Noop is correctly silent (avoids inflating counters in single-instance mode). `pgHandle.acquiredAt` + `mgr` back-reference enable hold-duration histogram observation at Release and LeaseLost emission in the heartbeat path.

Config validation correctly rejects: unknown DeployMode, clustered+sqlite combination, non-positive intervals. The clustered+sqlite check is important — clustered mode requires Postgres because the advisory-lock primitive is PG-only.

`podID` derives from `$HOSTNAME` (k8s default) with `os.Hostname()` fallback — no new config field needed. Sensible default.
