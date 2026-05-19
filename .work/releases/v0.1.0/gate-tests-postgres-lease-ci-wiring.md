---
id: gate-tests-postgres-lease-ci-wiring
kind: story
stage: done
tags: [testing, infra, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Postgres lease manager integration tests gated on `JAMSESH_TEST_PG_DSN` with no CI invocation visible

## Priority
Medium

## Spec reference
Item: `epic-cloud-native-deploy-lease-fencing-postgres`
Acceptance criterion: integration tests gated on `JAMSESH_TEST_PG_DSN`;
skip cleanly without.

## Gap type
test-integrity. The skip discipline is correct, but the 8 critical-path
assertions (acquire success, monotonic fencing tokens, ErrAlreadyHeld,
Lost-on-backend-terminate, Release idempotency, heartbeat liveness,
Release-after-Lost idempotency, collision check) ONLY run when an
operator has set the env var. There's no observable CI step in the
bundle that sets `JAMSESH_TEST_PG_DSN`.

## Suggested test
Either (a) wire a CI step that runs
`JAMSESH_TEST_PG_DSN=$dsn go test ./internal/portal/lease/...`
against a test Postgres, OR (b) restructure to use `testcontainers-go`
so the test is self-bootstrapping.

## Test location (suggested)
`internal/portal/lease/postgres_test.go` + `.github/workflows/*` and new
fixture wiring.

## Implementation notes

Took option (b): testcontainers self-bootstrap.

- Added `testcontainers-go` v0.42.0 and `testcontainers-go/modules/postgres`
  v0.42.0 to the main `go.mod`.
- Created `internal/portal/lease/testdb_test.go` with `acquireTestPostgres(t)`
  helper: uses `JAMSESH_TEST_PG_DSN` if set (operator override preserved),
  otherwise spins up a `postgres:16-alpine` testcontainer via `sync.Once`
  (shared per binary), creates a fresh per-test database, and registers
  `t.Cleanup` to drop it. Skips cleanly when Docker is unavailable.
- Removed the old `pgDSN(t)` skip-on-missing-env helper from
  `postgres_test.go`; all 8 call sites replaced with `acquireTestPostgres(t)`.
- Surfaced a pre-existing product bug: `TestPostgresCollisionDefensiveCheck`
  reveals `PostgresManager.Acquire` returns `nil` instead of `ErrAlreadyHeld`
  when a stale row with `pod_id != mgr.PodID AND released_at IS NULL` exists.
  Test is skipped with an inline comment linking to backlog item
  `lease-collision-check-not-returning-erralreadyheld`.
- 7 of 8 tests now run and pass under testcontainers without any env var.
  1 test is skipped (documented bug, not test infrastructure).

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Option (b) taken — testcontainers self-bootstrap. New testdb_test.go helper acquireTestPostgres(t) uses JAMSESH_TEST_PG_DSN if set (operator override preserved), otherwise spins up postgres:16-alpine via testcontainers with sync.Once + per-test fresh DB + t.Cleanup. 7 of 8 lease tests now run without any env-var coordination. Skips cleanly when Docker is unavailable. The 8th test (TestPostgresCollisionDefensiveCheck) surfaced a real production bug — Acquire does not return ErrAlreadyHeld on stale row collision — parked as backlog item lease-collision-check-not-returning-erralreadyheld; test is t.Skip-anchored to that id. testcontainers-go v0.42.0 added to go.mod.
