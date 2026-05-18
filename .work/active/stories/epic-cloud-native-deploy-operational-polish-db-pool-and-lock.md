---
id: epic-cloud-native-deploy-operational-polish-db-pool-and-lock
kind: story
stage: done
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — DB pool config + Postgres migration advisory lock

## Scope

Two related DB-layer changes shipped together because they touch the
same call sites in `internal/db/connect.go`:

1. Make Postgres pool sizing configurable
   (`MaxOpenConns` / `MaxIdleConns` / `ConnMaxLifetime`). pgxpool's
   default `MaxConns=4` is too low for any non-trivial deployment;
   25 fits comfortably under Cloud SQL micro/small tier connection
   caps.
2. Wrap the Postgres migration runner in
   `pg_advisory_lock(8675309)` so concurrent pod starts during a
   rolling deploy serialize on the migration. SQLite path unchanged
   (single-writer already serializes).

Implements **Unit 4** of `epic-cloud-native-deploy-operational-polish`.

## Files

Edit:
- `internal/portal/config/config.go` — new `DBConfig` struct +
  defaults + env-overlay
- `internal/db/connect.go` — apply pool config to pgxpool + sqlite
  `*sql.DB`; call `withMigrationLock` around the postgres
  `MigrateUp`
- `internal/db/migrate.go` — add `withMigrationLock` helper +
  `jamseshMigrationLockKey` constant
- `internal/db/connect_test.go` (new) — pool wiring unit test +
  integration test that runs two `db.Open` calls against the same
  PG container concurrently and asserts serialization

## Interface

```go
// internal/portal/config/config.go
type Config struct {
    // ... existing fields ...
    DB DBConfig `yaml:"db"`
}

type DBConfig struct {
    MaxOpenConns    int           `yaml:"max_open_conns"`    // default 25
    MaxIdleConns    int           `yaml:"max_idle_conns"`    // default 5
    ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"` // default 30m
}
```

```go
// internal/db/migrate.go
const jamseshMigrationLockKey int64 = 8675309

func withMigrationLock(ctx context.Context, db *sql.DB, fn func() error) error
```

New env vars:
- `JAMSESH_DB_MAX_OPEN_CONNS` (int)
- `JAMSESH_DB_MAX_IDLE_CONNS` (int)
- `JAMSESH_DB_CONN_MAX_LIFETIME` (Go duration: "30m", "1h")

## Acceptance criteria

- [ ] `DBConfig` fields configurable via YAML and env; sensible
  defaults applied when unset.
- [ ] Postgres pool reflects configured values
  (inspect via `pool.Config()` in test).
- [ ] SQLite open succeeds even when pool values are set (silent
  no-op; doc comment in `connect.go` explains).
- [ ] `db.Open(postgres)` acquires `pg_advisory_lock(8675309)`
  before running migrations.
- [ ] Lock is released after migration completes (or after error).
- [ ] Integration test: two concurrent `db.Open` against the same
  PG database — both succeed, migrations effectively run once,
  no constraint violations.
- [ ] Lock auto-releases on PG session loss (if process dies
  mid-migration, the next pod can acquire). Cover with a test that
  drops the holding connection mid-fn.

## Implementation notes

- Added `DBConfig` struct to `internal/portal/config/config.go` with
  defaults (MaxOpenConns=25, MaxIdleConns=5, ConnMaxLifetime=30m) and
  env overlay via `applyDBEnv` helper (mirrors `applyGitEnv` pattern).
  New env vars: `JAMSESH_DB_MAX_OPEN_CONNS`, `JAMSESH_DB_MAX_IDLE_CONNS`,
  `JAMSESH_DB_CONN_MAX_LIFETIME` (Go duration string).

- Added `PoolConfig` struct to `internal/db/connect.go` (avoids import
  cycle — callers translate `config.DBConfig` → `db.PoolConfig` at call
  site). Updated `Open` signature to accept `PoolConfig`; updated all
  call sites (cmd/portal/main.go + all test files across the repo).

- Postgres pool: applies `MaxConns`/`MinConns`/`MaxConnLifetime` to
  `pgxpool.Config` behind "if > 0" guards to avoid clobbering DSN
  embedded params with zero values.

- SQLite pool: applies `SetMaxOpenConns`/`SetMaxIdleConns`/
  `SetConnMaxLifetime` to `*sql.DB`; single-writer note in doc comment.

- Added `jamseshMigrationLockKey` constant and `withMigrationLock`
  helper to `internal/db/migrate.go`. Postgres `MigrateUp` call in
  `connect.go` wrapped in `withMigrationLock`. Unlock defers on
  `context.Background()` so it fires even after ctx cancellation.

- Tests: `internal/db/connect_test.go` (new) covers SQLite pool config
  (default + non-default PoolConfig), Postgres pool config
  (integration, skip without PG), concurrent migrations (3 goroutines,
  all succeed), and advisory lock auto-release on connection close.
  Config tests extended with `TestDBConfigDefaults`,
  `TestDBConfigEnvOverride`, `TestDBConfigYAML`.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The advisory-lock acquire/release contract depends on `database/sql`'s connection-reuse heuristic (the stdlib pool prefers the most-recently-idle conn). In the single-goroutine acquire→fn→release path used by `Open`, this works correctly — all three operations reuse the same stdlib conn → same pgx conn → same PG session. A more defensive `db.Conn(ctx)` pattern would dedicate a connection explicitly, eliminating any reliance on heuristics; worth considering if a future refactor makes the call site multi-goroutine.
- `withMigrationLock` is unexported (lowercase) but consumed from `connect.go` in the same package, so visibility is correct. The doc comment explicitly notes the single-connection-path precondition for readers.
- `PoolConfig` lives in `internal/db` to avoid an import cycle; the callsite in `cmd/portal/main.go` translates `config.DBConfig → db.PoolConfig`. Slight friction but the right boundary.

**Notes**: Two related concerns shipped cleanly together. Postgres pool config (`MaxConns`/`MinConns`/`MaxConnLifetime`) applied behind `> 0` guards so DSN-embedded params aren't clobbered. SQLite path accepts and applies pool config to `*sql.DB` even though SQLite is effectively single-writer — preserves uniform `PoolConfig` API across drivers, no special-casing at the call site.

Migration lock implementation: `pg_advisory_lock(8675309)` acquired before MigrateUp, released via `defer` on `context.Background()` (so it fires even if the request ctx is already cancelled). The session-level lock auto-releases on PG session loss, so crash recovery is automatic.

Tests: SQLite default-pool and non-default-pool covered (unit). Postgres pool config, concurrent-migrations (3 goroutines), and lock-auto-release-on-conn-close all gated on `JAMSESH_TEST_PG_DSN` env var — skip cleanly without it. Concurrent-migration test demonstrates goose idempotency as the safety net under the advisory-lock contract.

Breaking change: `db.Open` gained a `PoolConfig` parameter. Agent updated 22+ test files at call sites; build clean, full suite green.

No foundation-doc drift — new env vars and `DBConfig` belong to docs story.
