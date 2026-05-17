package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"jamsesh/internal/db/store"
)

// PoolConfig carries database connection pool tuning values.
// Defined here (not in internal/portal/config) to keep internal/db free of
// import cycles — callers translate their own config struct at the call site.
type PoolConfig struct {
	// MaxOpenConns is the maximum number of open connections.
	// For Postgres this maps to pgxpool.Config.MaxConns. Default 25.
	MaxOpenConns int
	// MaxIdleConns is the minimum number of idle connections the pool keeps.
	// For Postgres this maps to pgxpool.Config.MinConns. Default 5.
	MaxIdleConns int
	// ConnMaxLifetime caps how long a pooled connection lives before
	// being replaced. Default 30m.
	ConnMaxLifetime time.Duration
}

// Open opens a database connection for the given driver and DSN, runs pending
// migrations, and returns a ready-to-use Store.
//
// driver must be one of "sqlite" or "postgres".
//
// SQLite DSN notes:
//   - If the DSN does not already contain _pragma=foreign_keys, foreign key
//     enforcement is added automatically. FK enforcement is off by default in
//     SQLite; the portal requires it for ON DELETE CASCADE correctness.
//   - _pragma=busy_timeout(5000) is also injected unless already present, to
//     smooth over brief write-lock contention in concurrent tests.
//   - SQLite is effectively single-writer, so MaxOpenConns > 1 provides no
//     concurrency benefit. However the pool values are still applied to
//     *sql.DB so that callers can pass a unified PoolConfig without special-
//     casing the driver.
//
// Postgres DSN notes:
//   - Migrations are run via a temporary *sql.DB opened from the pgxpool via
//     pgx/v5/stdlib. The temporary connection is closed after migration; the
//     runtime pool remains open and is handed to the adapter.
//   - Migrations are serialised across concurrent pod starts with
//     pg_advisory_lock(8675309). The lock is automatically released when the
//     migration connection closes, so a pod crash cannot leave the lock
//     permanently held.
func Open(ctx context.Context, driver, dsn string, pc PoolConfig) (store.Store, error) {
	switch driver {
	case "sqlite":
		fullDSN := sqliteDSN(dsn)
		db, err := sql.Open("sqlite", fullDSN)
		if err != nil {
			return nil, fmt.Errorf("db: open sqlite (%s): %w", dsn, err)
		}
		// Apply pool settings. SQLite is effectively single-writer so
		// MaxOpenConns > 1 provides no concurrency benefit, but accepting
		// the values lets callers pass a unified PoolConfig without special-
		// casing the driver.
		if pc.MaxOpenConns > 0 {
			db.SetMaxOpenConns(pc.MaxOpenConns)
		}
		if pc.MaxIdleConns > 0 {
			db.SetMaxIdleConns(pc.MaxIdleConns)
		}
		if pc.ConnMaxLifetime > 0 {
			db.SetConnMaxLifetime(pc.ConnMaxLifetime)
		}
		if err := MigrateUp(ctx, db, "sqlite"); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
		return store.NewSQLiteAdapter(db), nil

	case "postgres":
		pgcfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, fmt.Errorf("db: parse postgres dsn: %w", err)
		}
		// Apply pool sizing; only override when a non-zero value is provided
		// so that DSN-embedded parameters aren't silently clobbered.
		if pc.MaxOpenConns > 0 {
			pgcfg.MaxConns = int32(pc.MaxOpenConns)
		}
		if pc.MaxIdleConns > 0 {
			pgcfg.MinConns = int32(pc.MaxIdleConns)
		}
		if pc.ConnMaxLifetime > 0 {
			pgcfg.MaxConnLifetime = pc.ConnMaxLifetime
		}
		pool, err := pgxpool.NewWithConfig(ctx, pgcfg)
		if err != nil {
			return nil, fmt.Errorf("db: connect postgres: %w", err)
		}
		// Migrations use database/sql, not pgx native. Open a temporary
		// *sql.DB from the pool via the pgx stdlib bridge, run migrations
		// under an advisory lock, then close it. The runtime pool stays open
		// for the adapter.
		mdb := stdlib.OpenDBFromPool(pool)
		if err := withMigrationLock(ctx, mdb, func() error {
			return MigrateUp(ctx, mdb, "postgres")
		}); err != nil {
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

// sqliteDSN appends required pragmas to a SQLite DSN if they are not already
// present. The modernc.org/sqlite driver accepts pragmas as query parameters
// using the _pragma=name(value) form.
func sqliteDSN(dsn string) string {
	// Separate file path from existing query parameters.
	path, query, _ := strings.Cut(dsn, "?")

	params := make([]string, 0, 3)
	if query != "" {
		params = append(params, query)
	}
	if !strings.Contains(query, "foreign_keys") {
		params = append(params, "_pragma=foreign_keys(1)")
	}
	if !strings.Contains(query, "busy_timeout") {
		params = append(params, "_pragma=busy_timeout(5000)")
	}

	if len(params) == 0 {
		return path
	}
	return path + "?" + strings.Join(params, "&")
}
