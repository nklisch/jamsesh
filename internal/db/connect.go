package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
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
// migrations, and returns a ready-to-use Store and the underlying *sql.DB.
//
// The *sql.DB is required by components that need a dedicated connection (e.g.
// the lease.PostgresManager which holds a dedicated *sql.Conn per active lease
// for Postgres session-scoped advisory locks). For SQLite the returned *sql.DB
// is the same one backing the adapter. Callers that do not need the *sql.DB
// can discard the second return value.
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
//   - The returned *sql.DB is a stdlib-bridge view of the pgxpool, suitable
//     for database/sql-style operations such as db.Conn(ctx) for dedicated
//     connections.
func Open(ctx context.Context, driver, dsn string, pc PoolConfig) (store.Store, *sql.DB, error) {
	switch driver {
	case "sqlite":
		fullDSN := sqliteDSN(dsn)
		db, err := sql.Open("sqlite", fullDSN)
		if err != nil {
			return nil, nil, fmt.Errorf("db: open sqlite (%s): %w", dsn, err)
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
			return nil, nil, fmt.Errorf("migrate: %w", err)
		}
		// Harden the on-disk file to 0600 (owner-only read/write). This
		// prevents other local users or container processes from reading the
		// database file, which holds oauth_tokens, magic_link_tokens, and
		// account email PII. chmod is best-effort: failure logs a warning but
		// does not abort startup. In-memory DSNs (:memory: / file::memory:)
		// have no file and are skipped.
		if fp := sqliteFilePath(dsn); fp != "" {
			if err := os.Chmod(fp, 0600); err != nil {
				slog.WarnContext(ctx, "db: sqlite chmod 0600 failed — DB file may be world-readable",
					"path", fp, "err", err)
			}
		}
		return store.NewSQLiteAdapter(db), db, nil

	case "postgres":
		pgcfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, nil, fmt.Errorf("db: parse postgres dsn: %w", err)
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
			return nil, nil, fmt.Errorf("db: connect postgres: %w", err)
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
			return nil, nil, fmt.Errorf("migrate: %w", err)
		}
		mdb.Close()
		// Open a runtime *sql.DB from the same pool. This is a thin stdlib
		// bridge — all connections still come from the pgxpool.
		runtimeDB := stdlib.OpenDBFromPool(pool)
		return store.NewPostgresAdapter(pool), runtimeDB, nil

	default:
		return nil, nil, fmt.Errorf("db: unknown driver %q", driver)
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
	if !strings.Contains(query, "_txlock") {
		params = append(params, "_txlock=immediate")
	}

	if len(params) == 0 {
		return path
	}
	return path + "?" + strings.Join(params, "&")
}

// sqliteFilePath extracts the filesystem path from a SQLite DSN, stripping any
// query parameters. Returns an empty string for in-memory DSNs (:memory: or
// file::memory:) since those have no on-disk file to chmod.
func sqliteFilePath(dsn string) string {
	// Strip any query parameters to get the raw path portion.
	path, _, _ := strings.Cut(dsn, "?")

	// Strip the "file:" URI prefix if present.
	path = strings.TrimPrefix(path, "file:")

	// In-memory DSNs have no file.
	if path == "" || path == ":memory:" {
		return ""
	}
	return path
}
