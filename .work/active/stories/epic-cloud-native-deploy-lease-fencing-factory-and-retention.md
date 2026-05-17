---
id: epic-cloud-native-deploy-lease-fencing-factory-and-retention
kind: story
stage: implementing
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
