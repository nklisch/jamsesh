---
name: sqlc
description: Reference for sqlc — type-safe Go generated from raw SQL. Auto-loads when editing sqlc.yaml, *.sql query files under db/queries/, db/schema/, files importing sqlitestore or pgstore packages, or when generated code with `-- name:` query annotations is in scope. Also triggers on terms — sqlc generate, sqlc compile, sqlc vet, WithTx, Querier, DBTX, emit_interface, sql_package, pgx/v5, dual-dialect, org_id discipline.
user-invocable: false
---

# sqlc reference (jamsesh)

**Pinned version**: v1.31.1 (2026-04-22). CLI: `github.com/sqlc-dev/sqlc`.
Generated code has zero runtime sqlc dependency.

## Project conventions (LOCKED)

jamsesh runs **dual-dialect**: SQLite (default, self-host) and Postgres
(scale-out swap). sqlc emits **two parallel packages** —
`db/sqlitestore/` and `db/pgstore/` — with isomorphic surfaces. A
`Store` interface wraps both; the runtime selects via config.

**org_id WHERE-clause discipline** (from `epic-portal-foundation`): every
query against an org-scoped table accepts `org_id` as a parameter and
includes it in WHERE. Code review enforces this. Missing-`org_id`
queries are a security bug.

## sqlc.yaml v2 (the canonical shape)

```yaml
version: "2"
sql:
  - engine: sqlite
    schema: db/schema/sqlite.sql
    queries: db/queries/sqlite/
    gen:
      go:
        package: sqlitestore
        out: db/sqlitestore
        emit_interface: true        # generates Querier
        emit_json_tags: true
        emit_prepared_queries: false
  - engine: postgresql
    schema: db/schema/postgres.sql
    queries: db/queries/postgres/
    gen:
      go:
        package: pgstore
        out: db/pgstore
        sql_package: pgx/v5         # IMPORTANT for Postgres
        emit_interface: true
        emit_json_tags: true
```

**Notes**:
- One `sql:` block per dialect — they share NOTHING at the sqlc layer.
- The two query directories typically hold the same query names with
  per-dialect placeholders (`?` vs `$1`) and any divergent syntax
  (`ON CONFLICT` vs `INSERT OR REPLACE`, `RETURNING` vs none, etc.).
- `sql_package: pgx/v5` makes the Postgres package emit pgx types
  (`pgtype.UUID`, `pgtype.Text`, `pgtype.Timestamptz`) and the
  `DBTX` interface matches pgx's `Conn`/`Tx`/`Pool`.

## Query file shape

```sql
-- name: GetSession :one
SELECT id, org_id, name, goal, created_at
FROM sessions
WHERE org_id = ? AND id = ?;   -- SQLite uses ?

-- name: ListSessionMembers :many
SELECT account_id, role
FROM session_members
WHERE org_id = ? AND session_id = ?
ORDER BY joined_at DESC;

-- name: CreateSession :execresult
INSERT INTO sessions (id, org_id, name, goal, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateSessionGoal :exec
UPDATE sessions SET goal = ? WHERE org_id = ? AND id = ?;
```

Annotations: `:one`, `:many`, `:exec`, `:execresult`, `:execrows`,
`:batchexec`, `:batchmany`, `:batchone` (pgx only), `:copyfrom` (pgx
only). `:one` errors with `sql.ErrNoRows` (sqlite) or `pgx.ErrNoRows`
(pgx) when no row matches.

## Generated code shape

```go
type DBTX interface {
    ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
    PrepareContext(context.Context, string) (*sql.Stmt, error)
    QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
    QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type Queries struct { db DBTX }
func New(db DBTX) *Queries { return &Queries{db: db} }
func (q *Queries) WithTx(tx *sql.Tx) *Queries { return &Queries{db: tx} }

func (q *Queries) GetSession(ctx context.Context, orgID, id string) (Session, error)
```

## Transactions

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback() // safe to call after Commit

qtx := queries.WithTx(tx)
if err := qtx.UpdateSessionGoal(ctx, "new goal", orgID, sessID); err != nil {
    return err
}
return tx.Commit()
```

For pgx: `tx, err := pool.Begin(ctx)`; `defer tx.Rollback(ctx)`;
`tx.Commit(ctx)`.

## The Store wrapper pattern (jamsesh-specific)

```go
package store

type Store interface {
    GetSession(ctx context.Context, orgID, id string) (Session, error)
    ListSessionMembers(ctx context.Context, orgID, sessionID string) ([]SessionMember, error)
    // ... one method per query
    WithTx(ctx context.Context, fn func(Store) error) error
}

type sqliteStore struct{ q *sqlitestore.Queries; db *sql.DB }
type pgStore struct{ q *pgstore.Queries; pool *pgxpool.Pool }

// Both implement Store. Handlers depend on Store only.
```

This keeps handler code dialect-agnostic and makes test-doubles
trivial.

## Common pitfalls

- **Placeholder mismatch**: copy-pasting a Postgres query (`$1`,
  `$2`) into the SQLite file silently breaks at runtime, not at
  generate. Use `sqlc vet` to catch syntax errors per engine.
- **`emit_json_tags`** generates `json:"snake_case"` from the column
  name. If your OpenAPI spec uses `camelCase`, do NOT serialize sqlc
  structs directly — convert to API types from `oapi-codegen`
  generated structs at the handler boundary.
- **`sql.NullString`** is the default for nullable columns under
  `database/sql`. Use `emit_pointers_for_null_types: true` (or
  per-column override `pointer: true`) to get `*string` instead.
  Pick one style project-wide.
- **pgx vs database/sql DBTX divergence**: a `Queries` generated with
  `sql_package: pgx/v5` is NOT interchangeable with one generated
  without — the DBTX interfaces differ. Don't try to share helpers.
- **Schema-only DDL**: sqlc parses `schema:` files for type
  inference; it does NOT run them. Use a separate migration tool
  (golang-migrate, atlas, goose) to actually apply schema.
- **org_id silently missing**: sqlc does NOT enforce that org_id is
  in WHERE. This is a code-review concern. Consider a linter rule or
  a `vet` rule for org-scoped tables.
- **`:execresult` vs `:exec`**: `:execresult` returns `sql.Result`
  (for `LastInsertId` in SQLite); `:exec` returns only `error`.

## Workflow

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
sqlc generate           # regenerate Go from current .sql files
sqlc vet                # static analysis (requires rules in sqlc.yaml)
sqlc compile            # parse without writing files (CI check)
```

Wire into `make generate` alongside `oapi-codegen` and
`openapi-typescript`. CI runs `make generate && git diff --exit-code`.

## References

- Foundation epic: `.work/active/epics/epic-portal-foundation.md`
  (data-layer child feature)
- Research doc: `docs/research/core-go-server-stack.md`
- Upstream docs: https://docs.sqlc.dev/en/latest/
- Repo: https://github.com/sqlc-dev/sqlc
