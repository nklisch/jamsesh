---
id: clustered-portal-concurrent-migration-race
kind: story
stage: implementing
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
