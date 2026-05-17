---
id: epic-cloud-native-deploy-operational-polish-db-pool-and-lock
kind: story
stage: implementing
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
