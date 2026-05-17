// Package db provides database migration helpers for the jamsesh portal.
// It supports two SQL dialects: SQLite (default, self-host) and Postgres
// (scale-out swap). Migration files are embedded into the binary via
// embed.FS so deployments require no external migration files on disk.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// sqliteMigrations embeds all SQLite migration SQL files.
// Resolved relative to this file: internal/db/migrations/sqlite/
//
//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

// postgresMigrations embeds all Postgres migration SQL files.
// Resolved relative to this file: internal/db/migrations/postgres/
//
//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

// MigrateUp applies all pending up-migrations for the given dialect against
// the provided *sql.DB. It is idempotent: running it against an
// already-current database is a no-op because goose tracks applied versions
// in the goose_db_version table.
//
// dialect must be one of "sqlite" or "postgres".
//
// SQLite note: migrations run without PRAGMA foreign_keys enforcement.
// The initial migration is CREATE TABLE only so no FK violations are
// possible. The runtime db.Open helper sets foreign_keys(1) in the DSN
// for all application connections; migration correctness does not depend
// on FK enforcement.
//
// Postgres note: pass a *sql.DB opened via pgx/v5/stdlib (not a pgxpool
// directly). The caller is responsible for its lifecycle; see
// internal/db/connect.go for the canonical pattern.
func MigrateUp(ctx context.Context, db *sql.DB, dialect string) error {
	var rawFS embed.FS
	var subDir string
	var gooseDialect goose.Dialect

	switch dialect {
	case "sqlite":
		rawFS = sqliteMigrations
		subDir = "migrations/sqlite"
		gooseDialect = goose.DialectSQLite3
	case "postgres":
		rawFS = postgresMigrations
		subDir = "migrations/postgres"
		gooseDialect = goose.DialectPostgres
	default:
		return fmt.Errorf("db: unknown dialect %q (want \"sqlite\" or \"postgres\")", dialect)
	}

	// fs.Sub strips the directory prefix so NewProvider sees migration files
	// at the root of the FS (e.g. "00001_initial.sql" not
	// "migrations/sqlite/00001_initial.sql").
	fsys, err := fs.Sub(rawFS, subDir)
	if err != nil {
		return fmt.Errorf("db: embed sub-FS (%s): %w", dialect, err)
	}

	// NewProvider creates a dialect-scoped migration runner backed by the
	// embedded FS. Using the Provider API avoids mutating package-level
	// globals (goose.SetDialect, goose.SetBaseFS), which would cause races
	// in tests that run both dialects concurrently.
	provider, err := goose.NewProvider(gooseDialect, db, fsys)
	if err != nil {
		return fmt.Errorf("db: goose provider init (%s): %w", dialect, err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("db: migrate up (%s): %w", dialect, err)
	}
	for _, r := range results {
		if r.Error != nil {
			return fmt.Errorf("db: migration %s failed: %w", r.Source.Path, r.Error)
		}
	}
	return nil
}
