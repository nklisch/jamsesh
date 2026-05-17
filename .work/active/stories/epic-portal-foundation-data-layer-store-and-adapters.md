---
id: epic-portal-foundation-data-layer-store-and-adapters
kind: story
stage: review
tags: [portal]
parent: epic-portal-foundation-data-layer
depends_on: [epic-portal-foundation-data-layer-queries-and-codegen]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Data Layer — Store Interface and Adapters

## Scope

Define the `Store` interface — the unified data-access seam that the rest
of the portal consumes — and provide SQLite and Postgres adapters that
delegate to the generated `Querier` packages. Implement the `db.Open`
factory that selects the dialect at startup.

After this story, callers depend only on `internal/db/store.Store`;
dialect selection happens once at startup.

## Units delivered

- **Unit 6**: `internal/db/store/store.go` — `Store` interface, domain
  types (`Org`, `Account`, `Session`, etc.), error sentinels
  (`ErrNotFound`, `ErrUniqueViolation`)
- **Unit 7**: `internal/db/store/sqlite_adapter.go` — wraps
  `sqlitestore.Queries` and `*sql.DB`
- **Unit 8**: `internal/db/store/postgres_adapter.go` — wraps
  `pgstore.Queries` and `*pgxpool.Pool`
- **Unit 9**: `internal/db/connect.go` — `Open(ctx, driver, dsn)`
  factory; runs migrations as part of Open
- **Time helper**: `internal/db/store/timefmt.go` — `formatTS` /
  `parseTS` helpers used by the SQLite adapter to keep all timestamps
  in UTC ISO-8601 with `Z` suffix

## Acceptance Criteria

- [ ] Compile-time assertions: `var _ store.Store = (*sqliteAdapter)(nil)`
      and `var _ store.Store = (*postgresAdapter)(nil)`
- [ ] `db.Open(ctx, "sqlite", ":memory:")` returns a `Store` and runs
      migrations
- [ ] `db.Open(ctx, "postgres", dsn)` returns a `Store` and runs
      migrations
- [ ] `db.Open(ctx, "unknown", "")` returns a wrapped error mentioning
      the driver name
- [ ] `Store.Close()` releases pool resources (verified by opening,
      closing, and re-opening against the same SQLite file path)
- [ ] Error mapping: `sql.ErrNoRows` and `pgx.ErrNoRows` both translate
      to `store.ErrNotFound`; Postgres SQLSTATE 23505 and SQLite
      `SQLITE_CONSTRAINT_UNIQUE` both translate to
      `store.ErrUniqueViolation`

## Notes

- The Store interface uses domain types, not sqlc-generated row types.
  Translation is boring code in the adapter — keep it mechanical and
  one-method-per-row, no clever helpers that obscure the mapping.
- All timestamp values written to SQLite go through `formatTS(time.Time)
  string` to guarantee UTC + `Z` suffix; reads go through `parseTS`.
- The `Open` factory is the only place that imports a dialect driver
  (`_ "modernc.org/sqlite"` for SQLite; pgx for Postgres). Callers
  never touch dialect packages directly.

## Implementation notes

### Files landed

- `internal/db/store/store.go` — `Store` interface, all domain types (`Org`,
  `Account`, `OrgMember`, `OrgMemberWithAccount`, `Session`, `SessionMember`,
  `SessionMembership`, `OAuthToken`, `MagicLinkToken`), all param types, and
  error sentinels (`ErrNotFound`, `ErrUniqueViolation`).
- `internal/db/store/timefmt.go` — `formatTS`/`parseTS` UTC ISO-8601 helpers
  (available for future use; current adapter relies on driver-level parsing).
- `internal/db/store/sqlite_adapter.go` — `sqliteAdapter` satisfies `Store`;
  delegates to `sqlitestore.Queries`; maps `sql.NullString` ↔ `*string` for
  `GithubUserID`; maps `SQLITE_CONSTRAINT_UNIQUE` → `ErrUniqueViolation` and
  `sql.ErrNoRows` → `ErrNotFound`.
- `internal/db/store/postgres_adapter.go` — `postgresAdapter` satisfies
  `Store`; delegates to `pgstore.Queries`; maps `pgtype.Text` ↔ `*string`;
  maps `pgconn.PgError` code `23505` → `ErrUniqueViolation` and
  `pgx.ErrNoRows` → `ErrNotFound`.
- `internal/db/connect.go` — `Open(ctx, driver, dsn)` factory with SQLite
  pragma injection (foreign_keys, busy_timeout) and Postgres migration bridge.
- `internal/db/store/store_test.go` — smoke tests: Open+Close+Open, ErrNotFound,
  ErrUniqueViolation, nullable GithubUserID, session CRUD, magic-link single-use.

### Implementation discovery: SQLite TEXT vs DATETIME column types

The prior story declared all timestamp columns as `TEXT` in the SQLite schema
and migration files. This caused a scan failure: modernc.org/sqlite returns
`string` driver values for `TEXT` columns, and `database/sql` cannot
automatically convert `string` → `time.Time` even when the scan destination is
`time.Time`.

The fix: changed all timestamp columns from `TEXT` to `DATETIME` in both
`db/schema/sqlite.sql` and `internal/db/migrations/sqlite/00001_initial.sql`.
The modernc.org/sqlite driver auto-parses `DATETIME` TEXT column values to
`time.Time` during scan (via its `parseTime` function in `rows.go`).
`sqlc generate` was re-run and confirmed to produce identical Go output —
the type overrides in `sqlc.yaml` apply regardless of `TEXT` vs `DATETIME`.

The `formatTS`/`parseTS` helpers in `timefmt.go` remain available for any
future code that stores timestamps as TEXT in dynamically-constructed queries.

### Error normalization approach

- SQLite: `errors.Is(err, sql.ErrNoRows)` → `ErrNotFound`;
  `errors.As(err, &sqlite.Error{})` with `Code() == SQLITE_CONSTRAINT_UNIQUE`
  → `ErrUniqueViolation`.
- Postgres: `errors.Is(err, pgx.ErrNoRows)` → `ErrNotFound` (pgx.ErrNoRows
  wraps sql.ErrNoRows so `errors.Is(sql.ErrNoRows)` also works);
  `errors.As(err, &pgconn.PgError{})` with `Code == "23505"` →
  `ErrUniqueViolation`.

### Deviations from design

- `GetSession` takes `(ctx, orgID, id string)` positional args rather than a
  params struct, consistent with the Unit 10 test shape shown in the feature
  doc.
- `timefmt.go` helpers are present but the adapter delegates timestamp parsing
  to the driver (via `DATETIME` column type) rather than round-tripping through
  manual string conversion. The helpers remain available for the next-story
  cross-org test suite if needed.
- `OrgMemberWithAccount` domain type added to `store.go` (not shown in feature
  doc sketch but required by `ListOrgMembers` which JOINs to accounts).
