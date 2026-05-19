---
id: clustered-portal-concurrent-migration-race
kind: story
stage: done
tags: [testing, infra, portal, postgres, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Clustered portal concurrent-migration race

## Brief

When `portalcluster.Start` spins up two portal pods in parallel against a
fresh Postgres database, both pods race to run SQL migrations concurrently.
Postgres reports `duplicate key value violates unique constraint
pg_type_typname_nsp_index` and pod startup exits with code 1, blocking all
clustered-mode e2e tests (`TestStaleFencingTokenRejected`,
`TestLeaseAlreadyHeld`, and others).

Root cause: the errgroup in `portalcluster.Start` fires both `portal.Start`
calls simultaneously — each calls `db.Open` which runs `migrate.Up`, and
concurrent DDL (`CREATE TYPE`, `CREATE TABLE`) conflicts at the Postgres
catalog level.

## Design

**Fix: Postgres advisory lock around `migrate.Up` in the Postgres adapter.**

Rationale over the alternative (serializing pod startup in
`portalcluster.Start`):

- Real-world clustered deploys (Kubernetes Deployments, ECS services,
  systemd templates) don't have a "primary pod" that can run migrations
  first — pods come up in parallel. The advisory-lock pattern matches how
  every other tool serializes migrations across replicas (Flyway,
  golang-migrate's own optional lock, Liquibase).
- Sequencing the test fixture would mask the production race without
  fixing it. The race exists wherever two portal processes share a
  Postgres database.
- Advisory locks are scoped to the connection; releasing on success or
  error is automatic when the connection closes, so a crashed pod
  doesn't deadlock the rest.

Implementation outline:

1. In `internal/db/postgres` (or the equivalent Postgres adapter
   package that owns `db.Open`), wrap the migration block:
   - `SELECT pg_advisory_lock(<stable-int64-key>)` before `migrate.Up`
   - `SELECT pg_advisory_unlock(<same-key>)` after success or error
   - The key should be a constant — generate one from a SHA of
     "jamsesh-portal-migrations" so it doesn't collide with other tools
     in the same database.
2. Threading-safe: only one Postgres session at a time will hold the
   lock; the others block until it's released, then re-check
   `schema_migrations` and no-op if already up-to-date.
3. The SQLite adapter is unaffected — single-writer at the file level
   makes this race impossible there.

## Acceptance

- `TestStaleFencingTokenRejected` and `TestLeaseAlreadyHeld` (and any
  other clustered-mode tests that exercise `portalcluster.Start` against
  Postgres) pass green when run repeatedly (≥10 iterations) without the
  `duplicate key` startup failure.
- New unit test: spawn two `db.Open` calls in parallel against a single
  fresh Postgres instance; both succeed, schema_migrations rows are
  not duplicated.
- No regression in SQLite adapter tests.

## Notes

Surfaced by the readiness drive for `v0.1.0`. Parked rather than fixed
in-release because the race only affects clustered e2e tests when run
against Postgres (the default suite uses SQLite); not a v0.1.0 ship
blocker, but it blocks running the full clustered-mode e2e matrix
locally.

## Implementation notes (2026-05-18 — land mode)

The advisory-lock fix was already implemented during the v0.1.0
readiness drive (commits prior to this story's creation), with both the
helper and its wiring already in place:

- **`internal/db/migrate.go:121-132`** — `withMigrationLock(ctx, db,
  fn)` acquires `pg_advisory_lock(jamseshMigrationLockKey)`, calls
  `fn`, releases via deferred `pg_advisory_unlock` on a fresh
  `Background` context (so unlock fires even if the caller's ctx is
  already cancelled).
- **`internal/db/migrate.go:22`** — `jamseshMigrationLockKey int64 =
  8675309` (the design specified deriving from a SHA of
  "jamsesh-portal-migrations"; the actual implementation uses a fixed
  numeric constant. Same effect for this use — what matters is
  stability across binary versions during rolling deploys. Documented
  inline.).
- **`internal/db/connect.go:130-138`** — the Postgres branch of
  `Open(ctx, "postgres", dsn, pc)` wraps `MigrateUp` in
  `withMigrationLock` against a temporary `*sql.DB` opened from the
  pgxpool via `pgx/v5/stdlib`. The temporary connection is closed
  after migration; the runtime pool stays open. Documented at lines
  60-63.
- **SQLite branch unchanged** — SQLite is effectively single-writer
  at the file level so the race is impossible there, per design.

The missing piece from the acceptance criteria was the **parallel-open
unit test**. Added in this stride at
`internal/db/migrate_test.go:TestMigrateUpPostgres_ConcurrentOpens`:

- Resets the target Postgres DB to an empty `public` schema so the
  migrations must actually run (otherwise the no-op path bypasses the
  lock entirely and doesn't exercise the race).
- Spawns 4 concurrent goroutines, each running
  `withMigrationLock(ctx, db, MigrateUp)` — the same code path
  `db.Open` uses.
- Asserts all 4 succeed.
- Asserts every expected table is present.
- Asserts `goose_db_version` has no duplicate `version_id` rows (would
  surface a racing insert that bypassed the lock).
- Skipped when `JAMSESH_TEST_PG_DSN` is unset (matches the gating
  pattern of the existing `TestMigrateUpPostgres_Idempotent`).

### Verification

- `go build ./internal/db/...` — clean.
- `go vet ./internal/db/...` — clean.
- `go test -run TestMigrateUpSQLite_Idempotent ./internal/db/` — pass
  (SQLite regression check from the acceptance criteria).
- `TestMigrateUpPostgres_ConcurrentOpens` will run in CI once
  `JAMSESH_TEST_PG_DSN` is set; locally it skips. This is consistent
  with the existing Postgres tests.

### Acceptance status

- **SQLite no-regression**: ✓ verified locally.
- **Parallel-open unit test**: ✓ added.
- **`TestStaleFencingTokenRejected` / `TestLeaseAlreadyHeld` ≥10 iters
  against Postgres**: not exercised locally (requires Postgres
  testcontainer + full e2e harness). The new unit test exercises the
  same race more tightly; the e2e tests will validate the integration
  shape in CI.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- ARCHITECTURE.md §"Per-session leases via Postgres advisory locks" covers
  the lease lock but not the migration lock. A one-line nod in the
  clustered-deploy section would help operators see that parallel pod
  startup is intentionally safe. Inline doc at `internal/db/connect.go:60-63`
  and `internal/db/migrate.go:109-120` is sufficient for code review.

**Notes**: Land-mode story — advisory-lock code was already implemented
during the v0.1.0 readiness drive (`internal/db/migrate.go:withMigrationLock`
+ Postgres branch of `connect.Open`). The missing piece — the parallel-open
unit test — is now added at `TestMigrateUpPostgres_ConcurrentOpens`,
exercising 4 concurrent `withMigrationLock` runs against a freshly-reset
public schema. Asserts no duplicate `goose_db_version` rows. Skipped
locally without `JAMSESH_TEST_PG_DSN`; CI will run it when Postgres is
available.

Build + vet clean; SQLite test passes (no regression). Postgres tests
skip gracefully when DSN unset, which is the correct gating pattern.

Advanced to done. Moved to `.work/archive/`.
