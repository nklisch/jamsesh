package lease_test

// acquireTestPostgres returns a Postgres DSN for use in integration tests.
//
// Priority:
//  1. If JAMSESH_TEST_PG_DSN is set, use it directly (operator override).
//  2. Otherwise spin up a postgres:16-alpine testcontainer.  The container is
//     shared for the lifetime of the test binary via sync.Once; a fresh
//     per-test database is created for isolation and dropped by t.Cleanup.
//
// The test is skipped (not failed) when Docker is unavailable so that
// `go test ./...` stays green on machines without Docker.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	sharedContainer *tcpostgres.PostgresContainer
	sharedBaseDSN   string
	containerOnce   sync.Once
	containerErr    error
)

// acquireTestPostgres returns a DSN pointing at a fresh per-test Postgres
// database, spinning up a testcontainer when no operator override is set.
func acquireTestPostgres(t *testing.T) string {
	t.Helper()

	// Operator override: skip testcontainer machinery entirely.
	if dsn := os.Getenv("JAMSESH_TEST_PG_DSN"); dsn != "" {
		return dsn
	}

	// Require Docker; skip cleanly if unavailable.
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("skipping Postgres lease integration test: Docker not available")
	}

	ctx := context.Background()

	containerOnce.Do(func() {
		c, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			containerErr = fmt.Errorf("start postgres container: %w", err)
			return
		}
		sharedContainer = c
		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			containerErr = fmt.Errorf("get postgres connection string: %w", err)
			return
		}
		sharedBaseDSN = dsn
	})

	if containerErr != nil {
		t.Skipf("skipping Postgres lease integration test: %v", containerErr)
	}

	// Create a per-test database for isolation.
	dbName := "leasetest_" + randTestHex(8)

	adminDB, err := sql.Open("pgx", sharedBaseDSN)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer adminDB.Close() //nolint:errcheck

	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
		t.Fatalf("create per-test database %q: %v", dbName, err)
	}

	host, err := sharedContainer.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	mappedPort, err := sharedContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("get container port: %v", err)
	}

	testDSN := fmt.Sprintf("postgres://test:test@%s:%d/%s?sslmode=disable",
		host, mappedPort.Num(), dbName)

	t.Cleanup(func() {
		dropCtx := context.Background()
		db, err := sql.Open("pgx", sharedBaseDSN)
		if err != nil {
			t.Logf("cleanup: open admin connection: %v", err)
			return
		}
		defer db.Close() //nolint:errcheck
		_, _ = db.ExecContext(dropCtx,
			fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()`, dbName),
		)
		if _, err := db.ExecContext(dropCtx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, dbName)); err != nil {
			t.Logf("cleanup: drop database %q: %v", dbName, err)
		}
	})

	return testDSN
}

func randTestHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}
