---
id: epic-portal-foundation-data-layer-store-and-adapters
kind: story
stage: implementing
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
