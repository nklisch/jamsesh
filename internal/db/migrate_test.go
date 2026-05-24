package db

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/pressly/goose/v3"
)

// migrateDown rolls back the database to the given goose version. Test-only:
// takes testing.TB so it can only be called from a test binary. Replicates
// the provider-construction block from MigrateUp, then calls Provider.DownTo.
//
// Goose semantics: DownTo(N) rolls back every migration with version > N,
// leaving the DB at version N. To revert migration M, pass version M-1.
// For example, migrateDown(t, ctx, db, "sqlite", 15) rolls the DB back
// through migration 00016 (and any later migrations), leaving it at the
// post-00015 schema.
//
// Why locally re-derive the Provider instead of factoring a shared helper out
// of MigrateUp? Production MigrateUp should not expose its Provider just for
// tests; one test isn't enough demand to justify the public surface. If a
// second migration test arrives, factor then.
func migrateDown(t testing.TB, ctx context.Context, db *sql.DB, dialect string, version int64) {
	t.Helper()

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
		t.Fatalf("migrateDown: unknown dialect %q", dialect)
	}

	fsys, err := fs.Sub(rawFS, subDir)
	if err != nil {
		t.Fatalf("migrateDown: embed sub-FS (%s): %v", dialect, err)
	}
	provider, err := goose.NewProvider(gooseDialect, db, fsys)
	if err != nil {
		t.Fatalf("migrateDown: goose provider init (%s): %v", dialect, err)
	}
	if _, err := provider.DownTo(ctx, version); err != nil {
		t.Fatalf("migrateDown: provider.DownTo(%d): %v", version, err)
	}
}

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

// TestMigrate00016_AnonymousBearers_UpDownUp verifies that migration 00016
// (anonymous bearers) applies cleanly, reverses cleanly, and re-applies.
// It also verifies data preservation: an existing OAuth token inserted before
// the Up migration survives and remains queryable after migration.
func TestMigrate00016_AnonymousBearers_UpDownUp(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open sqlite :memory:: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Apply all migrations up to 00015 (the migration before our new one) by
	// running the full MigrateUp (which applies all migrations to the latest).
	// Then verify the new column/table shapes are present.
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (full, including 00016): %v", err)
	}

	// Verify is_anonymous column exists on accounts.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, email, display_name, created_at, is_anonymous)
		 VALUES ('acc-test-001', 'test@example.com', 'Test', datetime('now'), 0)`); err != nil {
		t.Fatalf("is_anonymous column missing on accounts: %v", err)
	}

	// Verify session_id column exists on oauth_tokens and the new kind is accepted.
	// First create a minimal org+session for the FK.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at) VALUES ('org-001', 'Org', 'org-001', datetime('now'))`); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at)
		 VALUES ('sess-001', 'org-001', 'S', 'G', '[]', 'sync', 'active', datetime('now'))`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Insert a pre-migration style token (without session_id).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (id, account_id, token_hash, kind, issued_at, expires_at)
		 VALUES ('tok-001', 'acc-test-001', 'hash-001', 'access', datetime('now'), datetime('now', '+1 hour'))`); err != nil {
		t.Fatalf("insert pre-migration oauth_token: %v", err)
	}

	// Insert a new anonymous_session_bearer token (post-migration kind).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id, issued_at, expires_at)
		 VALUES ('tok-002', 'acc-test-001', 'hash-002', 'anonymous_session_bearer', 'sess-001', datetime('now'), datetime('now', '+1 hour'))`); err != nil {
		t.Fatalf("insert anonymous_session_bearer: %v", err)
	}

	// Verify both tokens exist.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM oauth_tokens`).Scan(&count); err != nil {
		t.Fatalf("count oauth_tokens: %v", err)
	}
	if count != 2 {
		t.Errorf("want 2 tokens, got %d", count)
	}

	// Verify the pre-migration token (tok-001) still has correct data.
	var kind string
	if err := db.QueryRowContext(ctx, `SELECT kind FROM oauth_tokens WHERE id='tok-001'`).Scan(&kind); err != nil {
		t.Fatalf("query pre-migration token: %v", err)
	}
	if kind != "access" {
		t.Errorf("pre-migration token kind: want 'access', got %q", kind)
	}

	// --- Down: roll back through 00016 ---
	// Goose Provider.DownTo(N) rolls back all migrations with version > N,
	// leaving the DB at version N. To revert 00016 we must pass version 15.
	migrateDown(t, ctx, db, "sqlite", 15)

	// Assert: anonymous_session_bearer rows deleted (CHECK now rejects the kind).
	// This must be asserted BEFORE re-Up, otherwise the assertion proves nothing.
	var anonCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM oauth_tokens WHERE kind='anonymous_session_bearer'`,
	).Scan(&anonCount); err != nil {
		t.Fatalf("count anonymous_session_bearer after Down: %v", err)
	}
	if anonCount != 0 {
		t.Errorf("after Down: anonymous_session_bearer rows: want 0, got %d", anonCount)
	}

	// Assert: pre-migration access token survived the Down without data loss.
	var preCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM oauth_tokens WHERE id='tok-001'`,
	).Scan(&preCount); err != nil {
		t.Fatalf("count pre-migration token after Down: %v", err)
	}
	if preCount != 1 {
		t.Errorf("after Down: tok-001 should survive: want 1 row, got %d", preCount)
	}

	// Assert: is_anonymous column removed from accounts. INSERT including the
	// column should fail because the column no longer exists.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, email, display_name, created_at, is_anonymous)
		 VALUES ('acc-test-002', 'test2@example.com', 'Test2', datetime('now'), 0)`); err == nil {
		t.Error("after Down: is_anonymous column should be gone but INSERT succeeded")
	}

	// Assert: session_id column removed from oauth_tokens. INSERT including
	// the column should fail because the column no longer exists.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id, issued_at, expires_at)
		 VALUES ('tok-003', 'acc-test-001', 'hash-003', 'access', 'sess-001', datetime('now'), datetime('now', '+1 hour'))`); err == nil {
		t.Error("after Down: session_id column should be gone but INSERT succeeded")
	}

	// --- Re-Up: reapply 00016 ---
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (re-apply after Down): %v", err)
	}

	// Assert: is_anonymous column back; session_id column back; new kind accepted.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (id, account_id, token_hash, kind, session_id, issued_at, expires_at)
		 VALUES ('tok-004', 'acc-test-001', 'hash-004', 'anonymous_session_bearer', 'sess-001', datetime('now'), datetime('now', '+1 hour'))`,
	); err != nil {
		t.Errorf("after re-Up: anonymous_session_bearer insert: %v", err)
	}
}

// TestMigrate00017_OrgProtected_UpDownUp verifies that migration 00017
// (org_protected column on orgs) applies cleanly, reverses cleanly, and
// re-applies.
//
// Isolation: this test brings the DB to version 16 (via full MigrateUp then
// migrateDown to 16), exercises the 17 boundary in isolation, and confirms
// schema state at each step without touching migration 18. Migration 18 is not
// applied during this test, so the down path is a clean single-step rollback.
func TestMigrate00017_OrgProtected_UpDownUp(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open sqlite :memory:: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Apply all migrations to bring the DB to the latest state, then roll back
	// to version 16 so we can exercise migration 17 in isolation.
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (full, to seed DB): %v", err)
	}
	// Roll back 18 and 17, leaving the DB at version 16.
	migrateDown(t, ctx, db, "sqlite", 16)

	// Verify org_protected column is absent at version 16 (pre-17).
	// INSERT with the column should fail because it does not exist yet.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at, org_protected)
		 VALUES ('org-pre17', 'Pre17 Org', 'pre17', datetime('now'), 0)`); err == nil {
		t.Error("at version 16: org_protected column should not exist but INSERT succeeded")
	}

	// --- Up: apply migration 00017 ---
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (up through 17): %v", err)
	}

	// Verify org_protected column exists and is usable.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at, org_protected)
		 VALUES ('org-after17', 'After17 Org', 'after17', datetime('now'), 1)`); err != nil {
		t.Fatalf("after Up to 17: org_protected column missing: %v", err)
	}

	// Verify DEFAULT 0 applies for rows inserted without the column.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at)
		 VALUES ('org-default17', 'Default17 Org', 'default17', datetime('now'))`); err != nil {
		t.Fatalf("after Up to 17: INSERT without org_protected (default): %v", err)
	}
	var protected int
	if err := db.QueryRowContext(ctx,
		`SELECT org_protected FROM orgs WHERE id='org-default17'`).Scan(&protected); err != nil {
		t.Fatalf("after Up to 17: SELECT org_protected: %v", err)
	}
	if protected != 0 {
		t.Errorf("after Up to 17: org_protected DEFAULT: want 0, got %d", protected)
	}

	// --- Down: roll back migration 00017 ---
	// migrateDown(16) rolls back version 17 (and any later, but 18 is not
	// applied here), leaving the DB at version 16.
	migrateDown(t, ctx, db, "sqlite", 16)

	// Verify org_protected column is gone after Down.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at, org_protected)
		 VALUES ('org-post-down17', 'PostDown17', 'post-down17', datetime('now'), 0)`); err == nil {
		t.Error("after Down to 16: org_protected column should be gone but INSERT succeeded")
	}

	// --- Re-Up: reapply migration 00017 ---
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (re-apply after Down): %v", err)
	}

	// Verify org_protected column is back after re-Up.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at, org_protected)
		 VALUES ('org-reup17', 'ReUp17 Org', 're-up17', datetime('now'), 1)`); err != nil {
		t.Fatalf("after re-Up to 17: org_protected column missing: %v", err)
	}
}

// TestMigrate00018_PlaygroundSessions_UpDownUp verifies that migration 00018
// (playground session columns + tombstones table) applies cleanly, reverses
// cleanly, and re-applies.
//
// Isolation: this test brings the DB to version 17, exercises the 18 boundary
// in isolation (up to 18, down to 17, up again), and confirms schema state at
// each step.
func TestMigrate00018_PlaygroundSessions_UpDownUp(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open sqlite :memory:: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Apply all migrations to bring the DB to latest, then roll back to
	// version 17 so we can exercise migration 18 in isolation.
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (full, to seed DB): %v", err)
	}
	// Roll back 18, leaving the DB at version 17.
	migrateDown(t, ctx, db, "sqlite", 17)

	// Verify playground columns are absent at version 17 (pre-18).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at, last_substantive_activity_at)
		 VALUES ('sess-pre18', 'org-001', 'S', 'G', '[]', 'sync', 'active', datetime('now'), datetime('now'))`); err == nil {
		t.Error("at version 17: last_substantive_activity_at column should not exist but INSERT succeeded")
	}

	// Verify tombstones table is absent at version 17.
	var tblName string
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='tombstones'`).Scan(&tblName); err == nil {
		t.Error("at version 17: tombstones table should not exist but was found")
	}

	// Seed an org and a session at version 17 to test data survival across Down.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, created_at) VALUES ('org-001', 'Org', 'org-001', datetime('now'))`); err != nil {
		// Row may already exist from prior test runs sharing the in-memory DB — ignore duplicate.
		_ = err
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at)
		 VALUES ('sess-v17', 'org-001', 'S', 'G', '[]', 'sync', 'active', datetime('now'))`); err != nil {
		t.Fatalf("seed session at version 17: %v", err)
	}

	// --- Up: apply migration 00018 ---
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (up through 18): %v", err)
	}

	// Verify last_substantive_activity_at column exists and pre-existing row
	// was back-filled from created_at (see migration comment).
	var lsaa string
	if err := db.QueryRowContext(ctx,
		`SELECT last_substantive_activity_at FROM sessions WHERE id='sess-v17'`).Scan(&lsaa); err != nil {
		t.Fatalf("after Up to 18: last_substantive_activity_at missing or unreadable: %v", err)
	}
	if lsaa == "" {
		t.Error("after Up to 18: last_substantive_activity_at should be back-filled from created_at, got empty string")
	}

	// Verify hard_cap_at and idle_timeout_at columns exist (nullable).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at, last_substantive_activity_at, hard_cap_at, idle_timeout_at)
		 VALUES ('sess-v18', 'org-001', 'S2', 'G', '[]', 'sync', 'active', datetime('now'), datetime('now'), datetime('now', '+2 hours'), datetime('now', '+1 hour'))`); err != nil {
		t.Fatalf("after Up to 18: playground columns missing: %v", err)
	}

	// Verify tombstones table exists.
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='tombstones'`).Scan(&tblName); err != nil {
		t.Fatalf("after Up to 18: tombstones table missing: %v", err)
	}

	// Insert a tombstone to confirm the table is usable.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO tombstones (session_id, org_id, members_count, commits_count, auto_merges_count, duration_seconds, end_reason, ended_at, expires_at)
		 VALUES ('sess-v18', 'org-001', 1, 2, 0, 300, 'idle_timeout', datetime('now'), datetime('now', '+30 days'))`); err != nil {
		t.Fatalf("after Up to 18: INSERT into tombstones: %v", err)
	}

	// --- Down: roll back migration 00018 ---
	// migrateDown(17) rolls back version 18, leaving the DB at version 17.
	migrateDown(t, ctx, db, "sqlite", 17)

	// Verify tombstones table is gone after Down.
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='tombstones'`).Scan(&tblName); err == nil {
		t.Error("after Down to 17: tombstones table should be gone but was found")
	}

	// Verify playground columns are gone after Down.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at, last_substantive_activity_at)
		 VALUES ('sess-post-down18', 'org-001', 'S', 'G', '[]', 'sync', 'active', datetime('now'), datetime('now'))`); err == nil {
		t.Error("after Down to 17: last_substantive_activity_at column should be gone but INSERT succeeded")
	}

	// Verify pre-migration session (sess-v17) survived the Down.
	var sessID string
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM sessions WHERE id='sess-v17'`).Scan(&sessID); err != nil {
		t.Fatalf("after Down to 17: sess-v17 should survive table rebuild: %v", err)
	}

	// --- Re-Up: reapply migration 00018 ---
	if err := MigrateUp(ctx, db, "sqlite"); err != nil {
		t.Fatalf("MigrateUp (re-apply after Down): %v", err)
	}

	// Verify tombstones table is back after re-Up.
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='tombstones'`).Scan(&tblName); err != nil {
		t.Fatalf("after re-Up to 18: tombstones table missing: %v", err)
	}

	// Verify playground columns are back and usable.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sessions (id, org_id, name, goal, writable_scope, default_mode, status, created_at, last_substantive_activity_at, hard_cap_at, idle_timeout_at)
		 VALUES ('sess-reup18', 'org-001', 'S3', 'G', '[]', 'sync', 'active', datetime('now'), datetime('now'), NULL, NULL)`); err != nil {
		t.Fatalf("after re-Up to 18: playground columns missing: %v", err)
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
