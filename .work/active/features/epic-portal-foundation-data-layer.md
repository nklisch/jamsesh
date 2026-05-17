---
id: epic-portal-foundation-data-layer
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-foundation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Data Layer

## Brief

The portal's persistence substrate. Establishes sqlc as the type system over
raw SQL, the dual-dialect pattern (SQLite default + Postgres via driver swap)
with per-dialect query packages selected at build or runtime, the initial
schema for the core auth entities (`orgs`, `accounts`, `sessions`, `members`,
`oauth_tokens`), and the org_id-in-WHERE convention that structurally
prevents cross-tenant leakage in every query.

This feature also delivers the migration tool (or convention) that brings a
fresh SQLite or Postgres database to the current schema, plus the connection
pool / driver setup helpers the HTTP skeleton's middleware will consume.

It does NOT cover schemas owned by other epics (`comments`,
`conflict_events`, `events`, `presence`, `invites` belong to
`epic-portal-api`; per-session repo storage on disk belongs to
`epic-portal-git`). It does NOT cover the HTTP-side middleware that uses
this data layer — that's the http-skeleton feature.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: linchpin feature — every other feature in this epic and
  every sibling epic that touches persistence depends on the sqlc patterns
  and the org_id discipline locked here.

## Foundation references

- `docs/SPEC.md` — Stack > Backend (sqlc, dual-dialect), Hard constraints
  (multi-tenant by design)
- `docs/ARCHITECTURE.md` — Data layer (multi-tenancy) section
- `docs/SECURITY.md` — Authorization > MCP and REST API authorization

## Inherited epic design decisions

The data layer inherits these decisions from epic-portal-foundation:

- **Token storage**: opaque random tokens, hashed at rest in `oauth_tokens`.
  No JWTs. Refresh tokens stored similarly; revocation is row deletion.
- **Multi-org per user**: the schema supports a many-to-many between
  accounts and orgs via the `members` table; "current org" is never
  stored — it's always taken from the URL path.
- **First-user bootstrap**: no special bootstrap state in the schema;
  first sign-in creates an org row and a member row like any other signup.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **SQLite driver**: `modernc.org/sqlite` (pure Go) — preserves the
  single-binary, no-cgo self-host story. cgo `mattn/go-sqlite3` was the
  alternative; rejected because it complicates cross-compilation and
  static linking.
- **Postgres driver**: `jackc/pgx/v5` with `pgxpool` — matches the
  research doc's sqlc dual-dialect example (`sql_package: pgx/v5`).
- **Migration tool**: `pressly/goose/v3` with `embed.FS` — embeds
  migrations into the binary, satisfies the single-binary install
  constraint, supports both SQLite and Postgres natively. Migrations are
  per-dialect directories (`db/migrations/sqlite/`,
  `db/migrations/postgres/`) kept in lockstep by review discipline.
- **ID format**: ULID strings (`github.com/oklog/ulid/v2`) — 26-char,
  sortable, URL-safe, dialect-agnostic (TEXT primary keys both sides).
- **Timestamps**: SQLite stores ISO-8601 strings in TEXT columns;
  Postgres uses TIMESTAMPTZ. sqlc per-engine type overrides map both
  to `time.Time`.
- **Schema scope (this feature)**: `orgs`, `accounts`, `org_members`,
  `sessions`, `session_members`, `oauth_tokens`, `magic_link_tokens`.
  Other tables (`comments`, `conflict_events`, `events`, `presence`,
  `invites`) are added by their owning features as separate query
  files in the same `sqlitestore` / `pgstore` packages.
- **Org-id discipline**: every query that selects from an org-scoped
  table includes `org_id = ?`/`$1` in its WHERE clause. Enforced by
  reviewer discipline + a cross-org-leakage test that runs against
  both dialects. `accounts`, `oauth_tokens`, and `magic_link_tokens`
  are account-scoped (or pre-account in the magic-link case) and
  carry no org_id.
- **Store abstraction**: hand-written `Store` interface in
  `internal/db/store/store.go`. Two adapters (`sqlitestore` and
  `pgstore`) implement it by delegating to the generated `Querier`
  for each dialect. Dialect selection is once-at-startup via a
  `New(driver, dsn) (Store, error)` factory.
- **Test strategy**: unit tests run against in-memory SQLite. An
  integration suite gated by `JAMSESH_TEST_PG_DSN` runs the same
  test cases against a Postgres instance (testcontainers-go is the
  reference path but a pre-provisioned DSN works too).

## Architectural choice

**Dual-package via sqlc `emit_interface: true`, unified by a hand-written
`Store` interface.**

The chosen approach matches the locked decision in `docs/SPEC.md` and the
patterns documented in `docs/research/core-go-server-stack.md`:

- `sqlc.yaml` declares two `sql:` blocks — one `engine: sqlite`, one
  `engine: postgresql` — each generating its own Go package with
  `emit_interface: true`.
- Generated packages: `internal/db/sqlitestore` (uses `database/sql`)
  and `internal/db/pgstore` (uses `pgx/v5`).
- Hand-written `Store` interface in `internal/db/store/store.go` exposes
  the methods the rest of the portal consumes. Each dialect package gets
  a thin adapter that satisfies `Store` by delegating to its generated
  `Querier`.
- The Adapter approach keeps dialect divergence at the call-site
  boundary — handlers and services depend only on `Store`. Queries
  remain real SQL in `.sql` files; sqlc handles type-safe codegen.

Alternatives considered:

- **Single package with dialect detection inside queries** — sqlc only
  emits one engine per `gen` block, so this is structurally
  unavailable.
- **Generated interface only, no hand-written Store** — would force
  every caller to pin to a specific dialect's generated package,
  defeating the swap.

## Implementation Units

### Unit 1: sqlc configuration

**File**: `sqlc.yaml`
**Story**: `epic-portal-foundation-data-layer-schema-and-migrations`

```yaml
version: "2"
sql:
  - engine: sqlite
    schema: db/schema/sqlite.sql
    queries: db/queries/sqlite
    gen:
      go:
        package: sqlitestore
        out: internal/db/sqlitestore
        emit_interface: true
        emit_json_tags: true
        emit_prepared_queries: false
        overrides:
          - column: "*.created_at"
            go_type: time.Time
          - column: "*.updated_at"
            go_type: time.Time
          - column: "*.issued_at"
            go_type: time.Time
          - column: "*.expires_at"
            go_type: time.Time
          - column: "*.last_used_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.revoked_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.used_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.ended_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.base_sha"
            go_type:
              type: string
              pointer: true
  - engine: postgresql
    schema: db/schema/postgres.sql
    queries: db/queries/postgres
    gen:
      go:
        package: pgstore
        out: internal/db/pgstore
        sql_package: pgx/v5
        emit_interface: true
        emit_json_tags: true
        overrides:
          - column: "*.last_used_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.revoked_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.used_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.ended_at"
            go_type:
              type: time.Time
              pointer: true
          - column: "*.base_sha"
            go_type:
              type: string
              pointer: true
```

**Acceptance Criteria**:
- [ ] `sqlc generate` runs clean against the schema + queries
- [ ] Both `internal/db/sqlitestore` and `internal/db/pgstore` packages
      are generated and committed
- [ ] Generated `Querier` interface is identical in shape across dialects
      (same method names, same parameter and return types)

---

### Unit 2: SQLite schema

**File**: `db/schema/sqlite.sql`
**Story**: `epic-portal-foundation-data-layer-schema-and-migrations`

```sql
CREATE TABLE orgs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    github_user_id TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE org_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    created_at TEXT NOT NULL,
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX org_members_account_idx ON org_members(account_id);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','ended','archived')),
    created_at TEXT NOT NULL,
    ended_at TEXT
);
CREATE INDEX sessions_org_idx ON sessions(org_id);

CREATE TABLE session_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    joined_at TEXT NOT NULL,
    PRIMARY KEY (session_id, account_id)
);
CREATE INDEX session_members_org_idx ON session_members(org_id);
CREATE INDEX session_members_account_idx ON session_members(account_id);

CREATE TABLE oauth_tokens (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh')),
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at TEXT
);
CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);

CREATE TABLE magic_link_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT
);
```

**Implementation Notes**:
- SQLite uses TEXT for timestamps (ISO-8601 in UTC). The sqlc overrides
  map these to `time.Time` in Go. Storage format MUST be UTC ISO-8601
  with `Z` suffix (`2026-05-16T19:30:00Z`) so SQLite's ordering
  comparisons sort correctly.
- `writable_scope` is a JSON array stored as TEXT; the data layer
  stores it verbatim. Parsing happens above the Store seam.
- Foreign keys require `PRAGMA foreign_keys = ON` per-connection;
  the SQLite open helper sets it.

---

### Unit 3: Postgres schema

**File**: `db/schema/postgres.sql`
**Story**: `epic-portal-foundation-data-layer-schema-and-migrations`

Same logical shape as SQLite, with PG-native types:

```sql
CREATE TABLE orgs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    github_user_id TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE org_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX org_members_account_idx ON org_members(account_id);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','ended','archived')),
    created_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ
);
CREATE INDEX sessions_org_idx ON sessions(org_id);

CREATE TABLE session_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    joined_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (session_id, account_id)
);
CREATE INDEX session_members_org_idx ON session_members(org_id);
CREATE INDEX session_members_account_idx ON session_members(account_id);

CREATE TABLE oauth_tokens (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh')),
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);

CREATE TABLE magic_link_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);
```

**Acceptance Criteria** (combined for Units 2 + 3):
- [ ] `sqlc generate` validates both schemas without errors
- [ ] Logical column set is identical across dialects (same columns,
      same nullability, same CHECK constraints)
- [ ] Indexes cover the org-scoped query paths: every `WHERE org_id = ?`
      query hits an index on (org_id) or (org_id, ...)

---

### Unit 4: Migration files

**Files**: `db/migrations/sqlite/00001_initial.sql`,
`db/migrations/postgres/00001_initial.sql`
**Story**: `epic-portal-foundation-data-layer-schema-and-migrations`

Goose-format migrations matching the schema files:

```sql
-- +goose Up
-- +goose StatementBegin
<table CREATE statements verbatim from schema file>
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS magic_link_tokens;
DROP TABLE IF EXISTS oauth_tokens;
DROP TABLE IF EXISTS session_members;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS org_members;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS orgs;
-- +goose StatementEnd
```

Migrations are embedded via `embed.FS` in `internal/db/migrate.go`:

```go
package db

import (
    "embed"
    "github.com/pressly/goose/v3"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

func MigrateUp(ctx context.Context, db *sql.DB, dialect string) error {
    var fsys embed.FS
    var goosed string
    switch dialect {
    case "sqlite":
        fsys = sqliteMigrations
        goosed = "sqlite3"
    case "postgres":
        fsys = postgresMigrations
        goosed = "postgres"
    default:
        return fmt.Errorf("db: unknown dialect %q", dialect)
    }
    goose.SetBaseFS(fsys)
    if err := goose.SetDialect(goosed); err != nil {
        return err
    }
    return goose.UpContext(ctx, db, "migrations/"+dialect)
}
```

**Implementation Notes**:
- Goose paths inside the embed.FS use forward slashes regardless of OS.
- The initial migration matches the schema file 1:1. Subsequent
  features add `00002_*.sql` etc. through the same directory.
- For Postgres we wrap the `*sql.DB` in goose; the runtime pool used by
  the app is `pgxpool`. Migration is the one place we open a vanilla
  `*sql.DB` against Postgres via `pgx/v5/stdlib`.

**Acceptance Criteria**:
- [ ] `MigrateUp` against an empty SQLite file brings the database to
      the schema
- [ ] `MigrateUp` against an empty Postgres database brings it to the
      schema
- [ ] Running `MigrateUp` twice is a no-op (idempotent)
- [ ] Down migration drops every table created by Up

---

### Unit 5: Query files

**Directories**: `db/queries/sqlite/`, `db/queries/postgres/`
**Story**: `epic-portal-foundation-data-layer-queries-and-codegen`

One `.sql` file per table per dialect. Files contain `-- name: ...`
annotated queries that sqlc compiles. The two dialects' files differ
only in placeholder style (`?` vs `$N`) where the SQL itself doesn't
diverge.

Initial query surface (names identical across dialects):

`orgs.sql`:
- `CreateOrg :one` — INSERT, RETURNING the row
- `GetOrgByID :one` — by ulid
- `GetOrgBySlug :one`

`accounts.sql`:
- `CreateAccount :one`
- `GetAccountByID :one`
- `GetAccountByEmail :one`
- `GetAccountByGitHubUserID :one`
- `UpdateAccountDisplayName :exec`

`org_members.sql`:
- `AddOrgMember :exec`
- `GetOrgMember :one` (org_id, account_id)
- `ListOrgsForAccount :many` (account_id) — joins to orgs
- `ListOrgMembers :many` (org_id) — joins to accounts
- `RemoveOrgMember :exec`

`sessions.sql`:
- `CreateSession :one` — org_id required
- `GetSession :one` — WHERE org_id = ? AND id = ?
- `ListSessionsForOrg :many` — WHERE org_id = ?
- `UpdateSessionStatus :exec` — WHERE org_id = ? AND id = ?
- `SetSessionBaseSHA :exec` — WHERE org_id = ? AND id = ?

`session_members.sql`:
- `AddSessionMember :exec`
- `GetSessionMember :one` — WHERE org_id = ? AND session_id = ?
                                AND account_id = ?
- `ListSessionMembers :many` — WHERE org_id = ? AND session_id = ?
- `RemoveSessionMember :exec` — WHERE org_id = ? AND session_id = ?
                                       AND account_id = ?
- `ListSessionMembershipsForAccount :many` — JOINs sessions to surface
  account-visible sessions across orgs (cross-org by design — used by
  `/api/me`-style endpoints)

`oauth_tokens.sql`:
- `CreateOAuthToken :one`
- `GetOAuthTokenByHash :one` — by token_hash; returns row including
  expires_at / revoked_at; callers check liveness
- `TouchOAuthTokenLastUsed :exec` — UPDATE last_used_at
- `RevokeOAuthToken :exec` — by id, sets revoked_at
- `RevokeAllOAuthTokensForAccount :exec` — account-wide kill switch
- `ListOAuthTokensForAccount :many`

`magic_link_tokens.sql`:
- `CreateMagicLinkToken :one`
- `GetMagicLinkTokenByHash :one`
- `ConsumeMagicLinkToken :exec` — UPDATE used_at WHERE id AND used_at
  IS NULL (single-use enforcement at SQL level)

**Implementation Notes**:
- Every `sessions` and `session_members` query carries `org_id` in
  WHERE. The cross-org-leak test (Unit 9) verifies this structurally.
- The `ListSessionMembershipsForAccount` query is an intentional
  exception: it spans orgs by joining through session_members ->
  sessions and returning each session's `org_id` to the caller. This
  is the only data-layer surface that walks the cross-org seam, and
  it does so for the authenticated account's own memberships, not for
  arbitrary lookups.

**Acceptance Criteria**:
- [ ] sqlc generates without error
- [ ] Every method on the generated `Querier` interface has identical
      signatures across `sqlitestore` and `pgstore` (verified by a
      compile-time interface satisfaction test in Unit 7)
- [ ] No query against `sessions` or `session_members` omits
      `org_id` from WHERE except the explicit
      `ListSessionMembershipsForAccount`

---

### Unit 6: Store interface

**File**: `internal/db/store/store.go`
**Story**: `epic-portal-foundation-data-layer-store-and-adapters`

```go
// Package store defines the data-access seam used by every component
// of the portal. Implementations are dialect-specific (sqlitestore,
// pgstore) and selected at startup by db.New(driver, dsn).
package store

import (
    "context"
    "time"
)

type Store interface {
    OrgStore
    AccountStore
    OrgMemberStore
    SessionStore
    SessionMemberStore
    OAuthTokenStore
    MagicLinkTokenStore

    // Close releases pool resources.
    Close() error
    // Dialect reports the underlying engine. Useful in logs / metrics.
    Dialect() string
}

type Org struct {
    ID        string
    Name      string
    Slug      string
    CreatedAt time.Time
}

type OrgStore interface {
    CreateOrg(ctx context.Context, arg CreateOrgParams) (Org, error)
    GetOrgByID(ctx context.Context, id string) (Org, error)
    GetOrgBySlug(ctx context.Context, slug string) (Org, error)
}

type CreateOrgParams struct {
    ID        string
    Name      string
    Slug      string
    CreatedAt time.Time
}

// ... analogous types and interfaces for Account, OrgMember, Session,
// SessionMember, OAuthToken, MagicLinkToken. The full set mirrors the
// sqlc-generated Querier method-by-method but expressed in domain
// types (no nullable sql.NullString, no engine-specific scalars).

// ErrNotFound is returned by Get* methods when the row is missing.
// Implementations translate sql.ErrNoRows / pgx.ErrNoRows into this.
var ErrNotFound = errors.New("store: not found")
```

**Implementation Notes**:
- The Store interface uses domain types (`Org`, `Account`, etc.), not
  the sqlc-generated row types. This insulates upper layers from
  dialect-specific scalar types and from naming churn in `.sql` files.
- The translation layer in each adapter converts between sqlc rows
  and domain types — boring code, but it's the seam that lets us swap
  engines.
- Errors are normalized: `sql.ErrNoRows`, `pgx.ErrNoRows` and
  Postgres unique-violation `23505` all map to `store.ErrNotFound` /
  `store.ErrUniqueViolation`.

---

### Unit 7: SQLite adapter

**File**: `internal/db/store/sqlite_adapter.go`
**Story**: `epic-portal-foundation-data-layer-store-and-adapters`

```go
package store

import (
    "context"
    "database/sql"
    "errors"

    "jamsesh/internal/db/sqlitestore"
)

type sqliteAdapter struct {
    q  *sqlitestore.Queries
    db *sql.DB
}

func newSQLiteAdapter(db *sql.DB) *sqliteAdapter {
    return &sqliteAdapter{q: sqlitestore.New(db), db: db}
}

func (a *sqliteAdapter) Dialect() string { return "sqlite" }
func (a *sqliteAdapter) Close() error    { return a.db.Close() }

func (a *sqliteAdapter) CreateOrg(ctx context.Context, p CreateOrgParams) (Org, error) {
    row, err := a.q.CreateOrg(ctx, sqlitestore.CreateOrgParams{
        ID: p.ID, Name: p.Name, Slug: p.Slug, CreatedAt: p.CreatedAt,
    })
    if err != nil {
        return Org{}, mapSQLiteErr(err)
    }
    return Org{ID: row.ID, Name: row.Name, Slug: row.Slug, CreatedAt: row.CreatedAt}, nil
}

// ... analogous wrappers per method.

func mapSQLiteErr(err error) error {
    if errors.Is(err, sql.ErrNoRows) {
        return ErrNotFound
    }
    // modernc.org/sqlite surfaces SQLITE_CONSTRAINT_UNIQUE via
    // *sqlite.Error; map to ErrUniqueViolation.
    return err
}
```

### Unit 8: Postgres adapter

**File**: `internal/db/store/postgres_adapter.go`
**Story**: `epic-portal-foundation-data-layer-store-and-adapters`

Same shape as the SQLite adapter but backed by `*pgxpool.Pool` and
`pgstore.Queries`. Error mapping handles `pgx.ErrNoRows` and the
Postgres SQLSTATE `23505` for unique violations.

### Unit 9: Connection helpers and factory

**File**: `internal/db/connect.go`
**Story**: `epic-portal-foundation-data-layer-store-and-adapters`

```go
package db

import (
    "context"
    "database/sql"
    "fmt"

    _ "modernc.org/sqlite"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5/stdlib"

    "jamsesh/internal/db/store"
)

// Open opens a database according to driver and runs migrations.
// driver: "sqlite" | "postgres"
func Open(ctx context.Context, driver, dsn string) (store.Store, error) {
    switch driver {
    case "sqlite":
        db, err := sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
        if err != nil {
            return nil, err
        }
        if err := MigrateUp(ctx, db, "sqlite"); err != nil {
            db.Close()
            return nil, fmt.Errorf("migrate: %w", err)
        }
        return store.NewSQLiteAdapter(db), nil
    case "postgres":
        cfg, err := pgxpool.ParseConfig(dsn)
        if err != nil {
            return nil, err
        }
        pool, err := pgxpool.NewWithConfig(ctx, cfg)
        if err != nil {
            return nil, err
        }
        // Migrations open a vanilla *sql.DB via pgx stdlib.
        mdb := stdlib.OpenDBFromPool(pool)
        if err := MigrateUp(ctx, mdb, "postgres"); err != nil {
            mdb.Close()
            pool.Close()
            return nil, fmt.Errorf("migrate: %w", err)
        }
        mdb.Close()
        return store.NewPostgresAdapter(pool), nil
    default:
        return nil, fmt.Errorf("db: unknown driver %q", driver)
    }
}
```

**Implementation Notes**:
- SQLite pragmas in the DSN: `foreign_keys(1)` is required for FK
  enforcement (off by default); `busy_timeout(5000)` smooths over
  brief lock contention on writes.
- Postgres migrations open and close a temporary `*sql.DB` from the
  pool, which keeps goose happy without changing the runtime pool.

**Acceptance Criteria** (combined for Units 6-9):
- [ ] `db.Open(ctx, "sqlite", ":memory:")` returns a working `Store`
- [ ] `db.Open(ctx, "postgres", dsn)` returns a working `Store`
- [ ] Both adapters satisfy the `Store` interface (compile-time
      assertion: `var _ store.Store = (*sqliteAdapter)(nil)`)
- [ ] Migrations run as part of `Open` and are idempotent

---

### Unit 10: Cross-org leakage test

**Files**: `internal/db/store/store_test.go`,
`internal/db/store/sqlite_test.go`, `internal/db/store/pg_test.go`
**Story**: `epic-portal-foundation-data-layer-org-id-tests`

A single parameterized test suite that runs against both dialects.
Two orgs are populated with sessions and session_members; queries
issued as one org must never return the other's rows.

```go
func TestOrgIDDiscipline(t *testing.T) {
    for _, tt := range stores(t) {
        tt := tt
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            s := tt.open(t)
            defer s.Close()

            orgA := mustCreateOrg(t, ctx, s, "org-a")
            orgB := mustCreateOrg(t, ctx, s, "org-b")
            accA := mustCreateAccount(t, ctx, s, "alice@example.com")

            mustAddOrgMember(t, ctx, s, orgA.ID, accA.ID, "creator")
            sessA := mustCreateSession(t, ctx, s, orgA.ID, "s-a")
            sessB := mustCreateSession(t, ctx, s, orgB.ID, "s-b")

            // Reading session B by org A must return ErrNotFound.
            _, err := s.GetSession(ctx, orgA.ID, sessB.ID)
            if !errors.Is(err, store.ErrNotFound) {
                t.Fatalf("expected ErrNotFound for cross-org read, got %v", err)
            }

            // Listing sessions for org A must not include session B.
            list, err := s.ListSessionsForOrg(ctx, orgA.ID)
            assertNoError(t, err)
            assertOnlyContains(t, list, sessA.ID)
        })
    }
}
```

The `stores(t)` helper enumerates `{sqlite, postgres}`, skipping
postgres if `JAMSESH_TEST_PG_DSN` is unset.

**Acceptance Criteria**:
- [ ] Test passes against SQLite in-memory
- [ ] Test passes against a running Postgres (CI flag-gated)
- [ ] Adding a new org-scoped table without org_id in WHERE causes a
      review-visible failure (the test is parameterized over the
      query surface so new omissions surface as new failing cases)

---

### Unit 11: Build wiring (Makefile + go.mod)

**Files**: `Makefile`, `go.mod`, `go.sum`
**Story**: `epic-portal-foundation-data-layer-queries-and-codegen`

`Makefile` target:

```makefile
.PHONY: generate generate-db
generate-db:
	sqlc generate
generate: generate-db
```

`go.mod` (initial):

```
module jamsesh

go 1.22

require (
    github.com/jackc/pgx/v5 v5.7.x
    github.com/oklog/ulid/v2 v2.x
    github.com/pressly/goose/v3 v3.x
    modernc.org/sqlite v1.x
)
```

CI verification:

```bash
make generate && git diff --exit-code
```

## Implementation Order

1. **schema-and-migrations** story — `sqlc.yaml`, schema files,
   migration files, migration runner, connection helpers stubs
2. **queries-and-codegen** story — query files for all 7 tables,
   `make generate`, generated `sqlitestore` + `pgstore` packages
   committed
3. **store-and-adapters** story — Store interface, SQLite adapter,
   Postgres adapter, factory
4. **org-id-tests** story — parameterized test suite, dialect
   enumeration helper, in-memory SQLite default + gated Postgres path

## Testing

### Unit Tests: `internal/db/store/*_test.go`

- **Cross-org leakage** (Unit 10) — the headline test
- **CRUD round-trips per table** — Create, Get, List, Update where
  applicable; both dialects
- **Constraint violations** — duplicate slug, missing FK, role CHECK
  failure; assert errors normalize correctly
- **Magic-link single-use** — `ConsumeMagicLinkToken` must succeed
  exactly once
- **OAuth token revocation** — revoked tokens return liveness=false
  via the application-side helper layered above the Store (the Store
  exposes the raw row; the helper is in `epic-portal-foundation-tokens`)

### Integration: `make generate` round-trip

CI runs `make generate && git diff --exit-code` to catch schema drift.

## Risks

- **Generated-type divergence between dialects.** sqlc may produce
  different Go types for identical-looking columns when timestamp
  representations differ. Mitigation: per-engine type overrides in
  `sqlc.yaml` (see Unit 1); spike the codegen output in the
  queries-and-codegen story and adjust overrides until both packages
  expose identical `Querier` signatures.
- **SQLite TEXT-timestamp ordering.** ISO-8601 sorts lexically only
  when always-UTC and always Z-suffixed. The adapter / writer layer
  must format consistently. Add a tiny helper `formatTS(t time.Time)
  string` in `internal/db/store/timefmt.go` and use it everywhere.
- **Migration drift between SQLite and Postgres.** Adding a column to
  one dialect's migrations and forgetting the other is the failure
  mode. Mitigation: each migration story-card requires both files to
  land together; CI checks file-count parity between the two
  `db/migrations/<dialect>/` directories.
- **goose + pgx interaction.** Goose uses `database/sql`, not pgx
  native. The temporary `*sql.DB` from `pgx/v5/stdlib.OpenDBFromPool`
  is correct but slightly subtle — leave a comment in `Open` noting
  why it's there.
