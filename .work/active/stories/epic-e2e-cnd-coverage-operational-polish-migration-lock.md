---
id: epic-e2e-cnd-coverage-operational-polish-migration-lock
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Migration concurrent-startup advisory lock coverage — failure

## Scope

One failure-mode test that starts 3 portal containers simultaneously
against a fresh per-test Postgres DB. Verifies the advisory-lock
serialization (`pg_advisory_lock(8675309)` per
`internal/db/migrate.go:17`) — exactly one portal runs migrations; the
others wait, then come up healthy.

Two-pronged assertion: (1) container-log inspection counts which pods
emit the migration-applied log line — exactly 1; (2) post-condition
query against `schema_migrations` (or equivalent table; library is
goose-or-similar — pin at impl time) verifies the schema is at a real
version.

## Files

- `tests/e2e/failure/migration_concurrent_startup_test.go`
- Possibly `tests/e2e/fixtures/containerlog/containerlog.go` (extension
  — add a synchronous `Capture` if only `DumpAndTerminate` exists; check
  before extending)

## Acceptance criteria

- [ ] Test starts N=3 portals in parallel via `errgroup.Group`
- [ ] All 3 report `/healthz` 200 (proven by `portal.Start`'s wait)
- [ ] Exactly 1 portal's container logs contain the "applying
      migrations" / "migration applied" log line (exact phrase pinned
      at impl time after a probe-run against a single-portal startup)
- [ ] Post-condition: `SELECT MAX(version) FROM
      <migrations_table>` returns > 0
- [ ] Test fails loudly if 0 OR 2+ portals report applying migrations
      (advisory-lock failure → split-brain on DDL is a real bug)

## Test integrity (from parent epic)

- The exact migration log phrase must be pinned by probing a real
  single-pod startup. Don't guess; read one real log output, then
  write the test.
- Migration table name is library-specific (goose, sqlx-migrate, etc.).
  Identify by reading `internal/db/migrate.go`'s migration library
  imports OR by querying `\dt` against a freshly-migrated DB.
- If the test sees 2+ portals migrating, that's a real Critical bug
  (advisory lock not serializing DDL). Park it, t.Skip with backlog
  id; do NOT loosen the assertion to "at least one".

## References

- Parent feature body, Unit 4 — full scaffold
- `internal/db/migrate.go:17,104,112-114` — advisory lock mechanism
- `internal/db/connect_test.go:73-100,185-207` — existing unit-level
  proof of advisory-lock behavior (useful background but not the e2e
  assertion target)
- `tests/e2e/fixtures/postgres/postgres.go` — per-test DB pattern
