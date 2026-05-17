// Package postgres provides a shared Testcontainers-Go fixture for Postgres.
//
// The fixture shares a single container per test binary (via sync.Once) and
// creates a fresh database per test invocation for isolation. The per-test
// database is automatically dropped by t.Cleanup.
//
// Usage:
//
//	pg := postgres.Start(ctx, t, postgres.Options{})
//	// pg.DSN is a postgres:// URL pointing at the per-test database
package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Postgres holds connection info for a per-test Postgres database.
type Postgres struct {
	// DSN is a postgres:// connection string for the per-test database.
	// This DSN uses the host-side (mapped) port and is suitable for test
	// process connections from outside Docker.
	DSN string

	// ContainerDSN is a postgres:// connection string that uses the
	// container's Docker bridge IP and the internal port (5432). Use this
	// when configuring another Docker container (e.g. the portal fixture) to
	// connect to Postgres — from inside Docker, the host-mapped port is not
	// reachable but the bridge IP is.
	ContainerDSN string

	Host string
	Port int
}

// Options configures the shared Postgres container.
// Reserved for future extension; all fields are optional.
type Options struct{}

var (
	shared    *tcpostgres.PostgresContainer
	baseDSN   string // points at the "test" default database (admin DB)
	once      sync.Once
	onceErr   error
)

// Start ensures the shared Postgres container is running (starting it if
// needed), creates a fresh per-test database, and returns connection info
// pointing at that database. t.Cleanup drops the database when the test ends.
func Start(ctx context.Context, t *testing.T, _ Options) *Postgres {
	t.Helper()
	requireDocker(t)

	once.Do(func() {
		c, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("test"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
		)
		if err != nil {
			onceErr = fmt.Errorf("postgres: start container: %w", err)
			return
		}
		shared = c
		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			onceErr = fmt.Errorf("postgres: get connection string: %w", err)
			return
		}
		baseDSN = dsn
	})

	if onceErr != nil {
		t.Fatalf("postgres: shared container failed: %v", onceErr)
	}

	// Create a per-test database using a random suffix for isolation.
	dbName := "test_" + randHex(8)

	adminDB, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("postgres: open admin connection: %v", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
		t.Fatalf("postgres: create per-test database %q: %v", dbName, err)
	}

	// Derive the per-test DSN from the base one by replacing the database name.
	host, err := shared.Host(ctx)
	if err != nil {
		t.Fatalf("postgres: get host: %v", err)
	}
	mappedPort, err := shared.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("postgres: get port: %v", err)
	}
	port := int(mappedPort.Num())

	testDSN := fmt.Sprintf("postgres://test:test@%s:%d/%s?sslmode=disable", host, port, dbName)

	// ContainerIP is the Docker bridge network IP of the Postgres container.
	// Other containers on the same default bridge network can reach Postgres
	// at this IP on port 5432 (no port mapping needed).
	containerIP, err := shared.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("postgres: get container IP: %v", err)
	}
	containerDSN := fmt.Sprintf("postgres://test:test@%s:5432/%s?sslmode=disable", containerIP, dbName)

	t.Cleanup(func() {
		dropCtx := context.Background()
		db, err := sql.Open("postgres", baseDSN)
		if err != nil {
			t.Logf("postgres: cleanup: open admin connection: %v", err)
			return
		}
		defer db.Close()
		// Terminate connections to the per-test DB before dropping.
		_, _ = db.ExecContext(dropCtx,
			fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()`, dbName),
		)
		if _, err := db.ExecContext(dropCtx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, dbName)); err != nil {
			t.Logf("postgres: cleanup: drop database %q: %v", dbName, err)
		}
	})

	return &Postgres{
		DSN:          testDSN,
		ContainerDSN: containerDSN,
		Host:         host,
		Port:         port,
	}
}

// randHex returns n hex-encoded random bytes (2*n characters).
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("postgres: rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}
