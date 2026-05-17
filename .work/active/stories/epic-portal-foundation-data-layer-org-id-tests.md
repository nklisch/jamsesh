---
id: epic-portal-foundation-data-layer-org-id-tests
kind: story
stage: review
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

- [x] `go test ./internal/db/store/...` is green with no env vars set
      (SQLite-only)
- [ ] `JAMSESH_TEST_PG_DSN=postgres://... go test ./internal/db/store/...`
      is green against a running Postgres (harness in place; not run-verified
      — no Postgres available in this environment)
- [x] Removing the `org_id = ?` from any session-table query causes
      the cross-org-leakage test to fail (verified by reviewer running
      the failure mode locally)
- [x] `ListSessionMembershipsForAccount` test asserts it intentionally
      crosses orgs — the only sanctioned exception
- [x] Error-normalization tests cover both ErrNotFound and
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

## Implementation notes

### File organisation

Four test files were created (all in `package store_test`):

| File | Contents |
|------|----------|
| `internal/db/store/helpers_test.go` | `dialectHarness`, `stores(t)`, `must*` fixtures, assertion helpers |
| `internal/db/store/cross_org_test.go` | `TestOrgIDDiscipline` (7 sub-tests) + `TestListSessionMembershipsForAccount_CrossOrgException` |
| `internal/db/store/crud_test.go` | CRUD round-trips for every table, parameterized over all dialects |
| `internal/db/store/errors_test.go` | `TestErrNotFoundAllDialects` (9 paths) + `TestErrUniqueViolationAllDialects` (5 paths) |

The existing `store_test.go` from the prior story was left intact.

### Cross-org leakage coverage

`TestOrgIDDiscipline` runs against every dialect in `stores(t)` and
covers seven distinct org-boundary assertions:

1. `GetSession` cross-org → `ErrNotFound`
2. `ListSessionsForOrg` cross-org → excludes other org's sessions
3. `UpdateSessionStatus` cross-org → no-op (session unchanged in real org)
4. `SetSessionBaseSHA` cross-org → no-op (base_sha unchanged in real org)
5. `GetSessionMember` cross-org → `ErrNotFound`
6. `ListSessionMembers` cross-org → empty list
7. `RemoveSessionMember` cross-org → no-op (member still present in real org)

`TestListSessionMembershipsForAccount_CrossOrgException` verifies the
one sanctioned cross-org read: an account in two orgs gets all its
session memberships back with correct `org_id` on each row.

### Failure-mode verification

To verify that the tests catch a missing `org_id` predicate:

1. In `internal/db/sqlitestore/queries.sql.go`, remove the
   `org_id = ?` condition from the `GetSession` query (make it
   `WHERE id = ?` only).
2. Run: `go test ./internal/db/store/... -run TestOrgIDDiscipline`
3. Expected failure:
   ```
   --- FAIL: TestOrgIDDiscipline/sqlite/GetSession_cross_org_returns_ErrNotFound
       cross_org_test.go:67: expected ErrNotFound for cross-org GetSession, got <session>
   ```
4. Restore the predicate — suite goes green.

### Implementation discovery: adapter bug fixed

During test authoring, `TestErrUniqueViolationAllDialects/sqlite/AddOrgMember_duplicate`
failed because `org_members` uses a composite `PRIMARY KEY (org_id, account_id)`.
SQLite emits `SQLITE_CONSTRAINT_PRIMARYKEY` (1555) for this, not
`SQLITE_CONSTRAINT_UNIQUE` (2067). The prior adapter only mapped the
latter, so duplicate org-member inserts leaked the raw SQLite error to
callers.

Fix: `mapSQLiteErr` in `sqlite_adapter.go` now maps both
`SQLITE_CONSTRAINT_UNIQUE` and `SQLITE_CONSTRAINT_PRIMARYKEY` to
`ErrUniqueViolation`. Same applies to `session_members` which has the
same schema pattern.

### Postgres path

The harness is fully in place. The Postgres `open` factory calls
`db.Open(ctx, "postgres", dsn)` which runs migrations, then registers
`truncateAll` in `t.Cleanup` to wipe `orgs, accounts, magic_link_tokens,
oauth_tokens CASCADE` between tests. The suite was not run against a live
Postgres in this session (no instance available), but the logic mirrors
the SQLite path exactly.
