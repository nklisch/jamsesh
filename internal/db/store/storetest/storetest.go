// Package storetest provides cross-dialect store fixtures usable from any
// _test package. SQLite is always available; Postgres is included only when
// JAMSESH_TEST_PG_DSN is set so local iteration stays fast.
//
// Typical use:
//
//	for _, h := range storetest.Stores(t) {
//	    h := h
//	    t.Run(h.Name, func(t *testing.T) {
//	        s := h.Open(t)
//	        // ... exercise the store against this dialect ...
//	    })
//	}
//
// The Postgres harness truncates all tables in dependency-safe order via
// CASCADE on cleanup so consecutive tests against a shared schema stay
// isolated. The schema is NOT dropped/recreated between calls — run
// migrations once before the suite (db.Open does this automatically on
// first connection).
//
// Concurrency note: do NOT call t.Parallel() inside the t.Run loop when
// using the Postgres harness against a shared DSN. The CASCADE truncate
// runs on t.Cleanup of every harness; concurrent test functions sharing
// the schema would race their cleanups. Rely on per-test SQLite :memory:
// for parallel speed instead.
package storetest

import (
	"context"
	"os"
	"testing"

	// pgx stdlib bridge — only used in truncateAll.
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

// DialectHarness bundles a dialect name and a factory that opens a fresh
// store for one test. The Open function registers a t.Cleanup to close
// (and for Postgres, truncate) the store so callers need not do it
// themselves.
type DialectHarness struct {
	Name string
	Open func(t *testing.T) store.Store
}

// Stores returns one harness per available dialect. SQLite is always
// present. Postgres is included only when JAMSESH_TEST_PG_DSN is set; it
// is skipped (not failed) when the env var is absent so local iteration
// remains fast.
func Stores(t *testing.T) []DialectHarness {
	t.Helper()

	var out []DialectHarness

	// SQLite: each call gets a fresh :memory: database with migrations
	// applied.
	out = append(out, DialectHarness{
		Name: "sqlite",
		Open: func(t *testing.T) store.Store {
			t.Helper()
			s, _, err := db.Open(context.Background(), "sqlite", ":memory:", db.PoolConfig{})
			if err != nil {
				t.Fatalf("open sqlite :memory:: %v", err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	})

	// Postgres: shared schema, TRUNCATE between calls for isolation.
	if dsn := os.Getenv("JAMSESH_TEST_PG_DSN"); dsn != "" {
		out = append(out, DialectHarness{
			Name: "postgres",
			Open: func(t *testing.T) store.Store {
				t.Helper()
				s, _, err := db.Open(context.Background(), "postgres", dsn, db.PoolConfig{})
				if err != nil {
					t.Fatalf("open postgres: %v", err)
				}
				t.Cleanup(func() {
					truncateAll(t, dsn)
					_ = s.Close()
				})
				return s
			},
		})
	}

	return out
}

// truncateAll clears all tables in dependency-safe order using a temporary
// *sql.DB opened from the pgx stdlib bridge. CASCADE handles FK children.
func truncateAll(t *testing.T, dsn string) {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Logf("storetest.truncateAll: parse dsn: %v", err)
		return
	}
	pool, err := pgxpool.New(context.Background(), cfg.ConnString())
	if err != nil {
		t.Logf("storetest.truncateAll: connect: %v", err)
		return
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	// Truncate root tables with CASCADE — child tables are handled
	// automatically.
	_, err = sqlDB.ExecContext(context.Background(),
		`TRUNCATE orgs, accounts, magic_link_tokens, oauth_tokens CASCADE`)
	if err != nil {
		t.Logf("storetest.truncateAll: truncate: %v", err)
	}
}
