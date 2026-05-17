---
id: epic-portal-foundation-data-layer-schema-and-migrations
kind: story
stage: implementing
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
