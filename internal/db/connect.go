package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"jamsesh/internal/db/store"
)

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
//
// Postgres DSN notes:
//   - Migrations are run via a temporary *sql.DB opened from the pgxpool via
//     pgx/v5/stdlib. The temporary connection is closed after migration; the
//     runtime pool remains open and is handed to the adapter.
func Open(ctx context.Context, driver, dsn string) (store.Store, error) {
	switch driver {
	case "sqlite":
		fullDSN := sqliteDSN(dsn)
		db, err := sql.Open("sqlite", fullDSN)
		if err != nil {
			return nil, fmt.Errorf("db: open sqlite (%s): %w", dsn, err)
		}
		if err := MigrateUp(ctx, db, "sqlite"); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
		return store.NewSQLiteAdapter(db), nil

	case "postgres":
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			return nil, fmt.Errorf("db: parse postgres dsn: %w", err)
		}
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("db: connect postgres: %w", err)
		}
		// Migrations use database/sql, not pgx native. Open a temporary
		// *sql.DB from the pool via the pgx stdlib bridge, run migrations,
		// then close it. The runtime pool stays open for the adapter.
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
