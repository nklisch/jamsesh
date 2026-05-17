package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// expectedTables is the set of application tables the initial migration
// creates. The goose_db_version table is created by goose itself and is
// not part of this list.
var expectedTables = []string{
	"orgs",
	"accounts",
	"org_members",
	"sessions",
	"session_members",
	"oauth_tokens",
	"magic_link_tokens",
	"leases",
}

// TestMigrateUpSQLite_Idempotent verifies that:
//   - MigrateUp creates all expected tables in a fresh SQLite :memory: DB.
//   - Calling MigrateUp a second time is a no-op (goose versioning).
func TestMigrateUpSQLite_Idempotent(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open sqlite :memory:: %v", err)
	}
	defer db.Close()

	// SQLite :memory: databases are connection-scoped; restrict to 1
	// connection so the schema is shared across all queries.
	db.SetMaxOpenConns(1)

	// First migration: must succeed and create all tables.
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (first call): %v", err)
	}

	assertSQLiteTables(t, db)

	// Second migration: must be a no-op with no error.
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (second call, idempotency): %v", err)
	}

	// Tables must still be there.
	assertSQLiteTables(t, db)
}

// TestMigrateUpPostgres_Idempotent verifies MigrateUp against a real Postgres
// instance. Requires JAMSESH_TEST_PG_DSN to be set; skipped otherwise.
func TestMigrateUpPostgres_Idempotent(t *testing.T) {
	dsn := os.Getenv("JAMSESH_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set JAMSESH_TEST_PG_DSN to enable Postgres migration tests")
	}

	ctx := context.Background()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open pgx: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	// First migration.
	if err := MigrateUp(ctx, db, "postgres"); err != nil {
		t.Fatalf("MigrateUp postgres (first call): %v", err)
	}

	assertPostgresTables(t, ctx, db)

	// Second migration: must be a no-op.
	if err := MigrateUp(ctx, db, "postgres"); err != nil {
		t.Fatalf("MigrateUp postgres (second call, idempotency): %v", err)
	}

	assertPostgresTables(t, ctx, db)
}

// assertSQLiteTables queries sqlite_master to verify all expected tables exist.
func assertSQLiteTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, tbl := range expectedTables {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist in sqlite_master: %v", tbl, err)
		}
	}
}

// assertPostgresTables queries information_schema to verify all expected
// tables exist.
func assertPostgresTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, tbl := range expectedTables {
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT table_name FROM information_schema.tables
             WHERE table_schema='public' AND table_name=$1`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist in postgres public schema: %v", tbl, err)
		}
	}
}
