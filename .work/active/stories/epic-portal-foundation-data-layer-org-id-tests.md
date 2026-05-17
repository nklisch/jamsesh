---
id: epic-portal-foundation-data-layer-org-id-tests
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-foundation-data-layer
depends_on: [epic-portal-foundation-data-layer-store-and-adapters]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Data Layer — org_id Discipline Tests

## Scope

The headline test for the data layer's structural multi-tenancy
guarantee. Parameterized over both dialects: a single test suite runs
against in-memory SQLite by default, and against Postgres when
`JAMSESH_TEST_PG_DSN` is set in the environment.

After this story, cross-org leakage in any future query change shows up
as a failing test, not as a runtime breach.

## Units delivered

- **Unit 10**: `internal/db/store/store_test.go` (cross-dialect
  shared cases) plus dialect-specific bootstrap files
  (`sqlite_test.go`, `pg_test.go`)
- Test helpers: `stores(t)` enumerates open-and-cleanup factories for
  each available dialect; `mustCreateOrg`, `mustCreateAccount`,
  `mustCreateSession` etc. for fixture setup
- CRUD round-trip tests for every table (Create + Get + List where
  applicable)
- Constraint-violation tests asserting error normalization
- Magic-link single-use test
- Cross-org leakage suite (the centerpiece)

## Acceptance Criteria

- [ ] `go test ./internal/db/store/...` is green with no env vars set
      (SQLite-only)
- [ ] `JAMSESH_TEST_PG_DSN=postgres://... go test ./internal/db/store/...`
      is green against a running Postgres
- [ ] Removing the `org_id = ?` from any session-table query causes
      the cross-org-leakage test to fail (verified by reviewer running
      the failure mode locally)
- [ ] `ListSessionMembershipsForAccount` test asserts it intentionally
      crosses orgs — the only sanctioned exception
- [ ] Error-normalization tests cover both ErrNotFound and
      ErrUniqueViolation paths in both dialects

## Notes

- The SQLite path uses `:memory:` per test to keep the suite fast and
  isolated. The pg path can share a connection if it migrates fresh
  per test using a `t.Cleanup` `TRUNCATE` cycle.
- The test helper `stores(t)` should skip the pg path with `t.Skip`
  (not fail) when `JAMSESH_TEST_PG_DSN` is unset, so local dev
  iteration stays painless.
- CI configures `JAMSESH_TEST_PG_DSN` against a service container so
  both paths run on every PR.
