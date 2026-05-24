# Pattern: Mirrored Dual-Dialect Query Files

For every table there are two `.sql` files with identical query names,
identical column lists, and identical `org_id`/`session_id` scoping —
one under `db/queries/sqlite/<table>.sql` (using `?` placeholders) and
one under `db/queries/postgres/<table>.sql` (using `$N` placeholders).
sqlc generates `sqlitestore` and `pgstore` packages from these;
per-dialect adapters in `internal/db/store/{sqlite_adapter,postgres_adapter}.go`
translate to the unified `store.Store` / `store.TxStore` interface.

## Rationale

The portal supports both dialects (SQLite for dev/single-tenant,
Postgres for clustered prod). Keeping queries verbatim mirrored — same
name, same WHERE shape, same column order — means the adapter layer is
mechanical and the `store.Store` interface is dialect-agnostic. Service
and handler code depends only on `store.Store`; dialect is chosen once
at startup via `db.Open(driver, dsn)`.

## Examples

### Example 1: sessions.sql — same operations, dialect-only delta

**File**: `db/queries/sqlite/sessions.sql:1`

```sql
-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, last_substantive_activity_at, hard_cap_at, idle_timeout_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;
```

**File**: `db/queries/postgres/sessions.sql:1`

```sql
-- name: CreateSession :one
INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, base_sha, status, created_at, ended_at, last_substantive_activity_at, hard_cap_at, idle_timeout_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;
```

### Example 2: identical file inventory

Both `db/queries/sqlite/` and `db/queries/postgres/` contain the same
18 files: `accounts, archived_sessions, comments, conflict_events,
events, finalize_locks, leases, magic_link_tokens, oauth_state,
oauth_tokens, org_invites, org_members, orgs, presence, ref_modes,
session_invites, session_members, sessions`.

### Example 3: org_id scoping is verbatim across dialects

**File**: `db/queries/sqlite/sessions.sql:18`

```sql
-- name: UpdateSessionStatus :exec
UPDATE sessions
SET status = ?
WHERE org_id = ? AND id = ?;
```

**File**: `db/queries/postgres/sessions.sql:18` — identical text,
`$1`/`$2`/`$3` placeholders. 32 occurrences of `org_id`/`session_id`
filters in each dialect's queries — counts match exactly.

## When to Use

- Adding a new query for an existing table — write it in both files,
  with the same name and WHERE shape.
- Adding a new table — add the schema migration to both
  `db/schema/{sqlite,postgres}.sql`, plus a parallel pair of query
  files.

## When NOT to Use

- A genuinely dialect-only operation (`IssueLeaseFencingToken` is
  Postgres-only; the SQLite adapter returns an error). Document the
  asymmetry in the `store.LeaseStore` interface doc comment.
- Read-side-only utilities that don't go through `store.Store` (e.g.
  ad-hoc migration scripts).

## Common Violations

- Adding a query to only one dialect — the unified `store.Store`
  interface compiles but the runtime adapter for the other dialect
  panics or returns `ErrNotImplemented`.
- Diverging the WHERE clause between dialects (e.g. forgetting
  `org_id` in one) — silent multi-tenant data leak in production while
  SQLite tests pass.
- Reordering RETURNING/SELECT columns between dialects — sqlc generates
  different struct layouts; adapter row mappers break.
