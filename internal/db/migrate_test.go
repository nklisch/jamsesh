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

// TestMigrateUpPostgres_ConcurrentOpens verifies that two db.Open calls racing
// against a single fresh Postgres database both succeed, with no
// "duplicate key value violates unique constraint pg_type_typname_nsp_index"
// or similar catalog-level conflict. This reproduces the failure mode that
// blocked clustered-mode e2e tests when two portal pods spun up in parallel:
// each pod called db.Open → MigrateUp simultaneously, and the concurrent
// DDL (CREATE TYPE, CREATE TABLE) conflicted at the Postgres catalog level.
//
// The fix is the pg_advisory_lock acquired in connect.Open's Postgres branch
// (see internal/db/connect.go:130-138). This test exercises that lock by
// spawning multiple goose providers concurrently against the same database
// and confirming all succeed and the goose_db_version table contains exactly
// one row per migration (no duplicates from racing inserts).
//
// Requires JAMSESH_TEST_PG_DSN to point at an EMPTY Postgres database
// (typically a Docker-managed test instance). The DB is reset to empty
// at the start of the test; do not point this at a database with other
// state you care about.
func TestMigrateUpPostgres_ConcurrentOpens(t *testing.T) {
	dsn := os.Getenv("JAMSESH_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("set JAMSESH_TEST_PG_DSN to enable Postgres migration tests")
	}

	ctx := context.Background()

	// Reset the target DB so the migrations have to actually run (a no-op
	// no-DDL path would not exercise the lock contention). Drop every
	// user-created object in the public schema. CASCADE clears dependent
	// objects (enums, sequences) introduced by future migrations.
	resetDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open pgx (reset): %v", err)
	}
	if _, err := resetDB.ExecContext(ctx,
		"DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		resetDB.Close()
		t.Fatalf("reset public schema: %v", err)
	}
	resetDB.Close()

	// Spawn N concurrent migration runs through the same code path that
	// db.Open uses: stdlib.OpenDBFromPool → withMigrationLock → MigrateUp.
	// We use raw sql.Open + the helper directly here to keep the test
	// inside this package and avoid pulling in the pgxpool dependency
	// dance; the lock semantics are identical because withMigrationLock
	// operates on any *sql.DB whose connections share a session.
	const concurrentOpens = 4
	errs := make(chan error, concurrentOpens)
	for i := 0; i < concurrentOpens; i++ {
		go func() {
			db, err := sql.Open("pgx", dsn)
			if err != nil {
				errs <- err
				return
			}
			// Restrict to a single connection so withMigrationLock's
			// session-scoped lock applies to MigrateUp's connections.
			db.SetMaxOpenConns(1)
			defer db.Close()
			errs <- withMigrationLock(ctx, db, func() error {
				return MigrateUp(ctx, db, "postgres")
			})
		}()
	}
	for i := 0; i < concurrentOpens; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent MigrateUp %d/%d: %v", i+1, concurrentOpens, err)
		}
	}

	// All tables must be present after the race resolves.
	verifyDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open pgx (verify): %v", err)
	}
	defer verifyDB.Close()

	assertPostgresTables(t, ctx, verifyDB)

	// goose_db_version must have exactly one row per applied migration.
	// Concurrent racing inserts on the same version would show up as
	// duplicate version_id values (goose does not constrain version_id
	// uniqueness on the table, so we check it explicitly here).
	rows, err := verifyDB.QueryContext(ctx,
		`SELECT version_id, COUNT(*) AS n
		 FROM goose_db_version
		 GROUP BY version_id
		 HAVING COUNT(*) > 1`)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version, count int64
		if err := rows.Scan(&version, &count); err != nil {
			t.Fatalf("scan duplicate row: %v", err)
		}
		t.Errorf("goose_db_version has %d rows for version %d (expected 1)",
			count, version)
	}
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
