package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestStartPostgres verifies that Start brings up the shared container, creates
// a per-test database, and that the database is reachable via the returned DSN.
func TestStartPostgres(t *testing.T) {
	ctx := context.Background()
	pg := postgres.Start(ctx, t, postgres.Options{})
	if pg.DSN == "" {
		t.Fatal("expected non-empty DSN")
	}
	if pg.Host == "" {
		t.Fatal("expected non-empty Host")
	}
	if pg.Port == 0 {
		t.Fatal("expected non-zero Port")
	}

	db, err := sql.Open("postgres", pg.DSN)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

// TestStartPostgresIsolation verifies that two calls to Start return different
// databases that share the same underlying container.
func TestStartPostgresIsolation(t *testing.T) {
	ctx := context.Background()
	pg1 := postgres.Start(ctx, t, postgres.Options{})
	pg2 := postgres.Start(ctx, t, postgres.Options{})
	if pg1.DSN == pg2.DSN {
		t.Fatalf("expected different DSNs for per-test isolation, got same: %s", pg1.DSN)
	}
	if pg1.Host != pg2.Host || pg1.Port != pg2.Port {
		t.Fatalf("expected same container host/port: %s:%d vs %s:%d", pg1.Host, pg1.Port, pg2.Host, pg2.Port)
	}
}
