---
id: epic-portal-foundation-data-layer-schema-and-migrations
kind: story
stage: review
tags: [portal]
parent: epic-portal-foundation-data-layer
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Data Layer — Schema and Migrations

## Scope

Bring up the persistence substrate's schema and migration tooling for both
SQLite and Postgres. After this story, a fresh empty database can be
brought to the current schema by calling `db.MigrateUp(ctx, db, dialect)`.

This is the foundation step in the data-layer feature. It does NOT include
queries (queries-and-codegen story) or the Store interface
(store-and-adapters story).

## Units delivered

- **Unit 1**: `sqlc.yaml` — dual-engine generator config (sqlite + postgres)
- **Unit 2**: `db/schema/sqlite.sql` — TEXT-timestamp schema
- **Unit 3**: `db/schema/postgres.sql` — TIMESTAMPTZ schema
- **Unit 4**: `db/migrations/sqlite/00001_initial.sql` and
  `db/migrations/postgres/00001_initial.sql` — goose-format migrations
  matching the schema files 1:1
- **Migration runner**: `internal/db/migrate.go` — `MigrateUp(ctx, *sql.DB, dialect)`
  using `pressly/goose/v3` with embedded `embed.FS`

## Module bootstrap

This story also creates `go.mod` (module name `jamsesh`, Go 1.22+) and
adds the initial dependencies: `pressly/goose/v3`, `modernc.org/sqlite`,
`jackc/pgx/v5`, `oklog/ulid/v2`.

## Acceptance Criteria

- [ ] `sqlc generate` runs clean (even though queries don't exist yet, the
      schema must validate)
- [ ] `MigrateUp` against an empty SQLite file (`:memory:` and on-disk)
      brings it to the schema
- [ ] `MigrateUp` against an empty Postgres database brings it to the
      schema (test gated on `JAMSESH_TEST_PG_DSN`)
- [ ] Running `MigrateUp` twice in a row is a no-op (goose's own
      versioning handles this; verify with a test)
- [ ] Down migrations cleanly drop every table created by Up
- [ ] Schema files and migration files declare the same logical columns
      in the same order (manual review checklist captured in the story
      body's review notes)

## Notes

- See parent feature body for the full schema text and `sqlc.yaml`.
- Foreign-key enforcement on SQLite requires `PRAGMA foreign_keys = ON`
  per-connection; the `db.Open` helper sets it via DSN pragmas. For
  migrations specifically, goose opens its own connection, so the
  SQLite migration helper must apply the same pragma before goose runs.

## Implementation notes

### Files landed

| File | Description |
|------|-------------|
| `sqlc.yaml` | Dual-dialect sqlc v2 config: sqlite + postgresql blocks, both with `emit_interface: true`, per-engine timestamp overrides, nullable-column pointer overrides |
| `db/schema/sqlite.sql` | Full 7-table schema with TEXT timestamps, CHECK constraints, FK references, indexes |
| `db/schema/postgres.sql` | Same logical schema with TIMESTAMPTZ, same constraints and indexes |
| `internal/db/migrations/sqlite/00001_initial.sql` | Goose Up/Down wrapping the SQLite schema statements |
| `internal/db/migrations/postgres/00001_initial.sql` | Goose Up/Down wrapping the Postgres schema statements |
| `internal/db/migrate.go` | `MigrateUp(ctx, *sql.DB, dialect)` using `goose.NewProvider` + `embed.FS` |
| `internal/db/migrate_test.go` | `TestMigrateUpSQLite_Idempotent` (passes), `TestMigrateUpPostgres_Idempotent` (skips without `JAMSESH_TEST_PG_DSN`) |
| `go.mod` / `go.sum` | Added: `pressly/goose/v3@v3.27.1`, `modernc.org/sqlite@v1.50.1`, `jackc/pgx/v5@v5.9.2`, `oklog/ulid/v2@v2.1.1` |

### Validation results

- `go test ./internal/db/...` — PASS (SQLite idempotency test passes; Postgres test skips cleanly)
- `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 compile` — clean, no output
- `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` — clean, generates `internal/db/sqlitestore/` and `internal/db/pgstore/` with models, querier, db files
- `go vet ./...` — clean

### Deviations from design

**Migration file location**: The feature design specifies `db/migrations/sqlite/` and `db/migrations/postgres/` at the repo root, but Go's `//go:embed` directive does not allow `..` in paths — the embed path must be relative to the source file and within its subtree. Since `migrate.go` lives at `internal/db/`, the migrations are placed at `internal/db/migrations/sqlite/` and `internal/db/migrations/postgres/`. Functionally identical; the embed path in the source (`migrations/sqlite/*.sql`) resolves correctly.

**sqlc.yaml output paths**: Feature design specifies `internal/db/sqlitestore` and `internal/db/pgstore` (not `db/sqlitestore` and `db/pgstore` as in the sqlc skill reference). Feature design takes precedence.

**Generated files not committed**: `sqlc generate` produces output in `internal/db/sqlitestore/` and `internal/db/pgstore/`, but these belong to the `queries-and-codegen` story. Only `.gitkeep` placeholder files are committed in those directories. The dummy `_validate.sql` query files remain in `db/queries/sqlite/` and `db/queries/postgres/` to enable `sqlc compile/generate` validation.

**FK enforcement in migrations**: Chose the simpler path — SQLite migrations run without `PRAGMA foreign_keys = ON`. The initial migration is CREATE TABLE only, so no FK violations are possible. Documented in `migrate.go` comments.

**goose API**: Used `goose.NewProvider` (v3 provider API) with `fs.Sub` to strip the embed path prefix, avoiding package-level global mutation (`goose.SetBaseFS`, `goose.SetDialect`). This is safe for concurrent test runs.

### Schema/migration column parity (review checklist)

Manually verified that `db/schema/sqlite.sql`, `db/schema/postgres.sql`,
`internal/db/migrations/sqlite/00001_initial.sql`, and
`internal/db/migrations/postgres/00001_initial.sql` declare identical
columns in the same order for all 7 tables. Only type differences
(TEXT vs TIMESTAMPTZ for timestamps) are expected and intentional.
