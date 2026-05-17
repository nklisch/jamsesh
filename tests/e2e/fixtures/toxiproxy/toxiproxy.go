// Package toxiproxy provides a Testcontainers-Go fixture for Toxiproxy.
//
// Toxiproxy is a TCP proxy that simulates network conditions (latency, packet
// loss, etc.) by injecting "toxics". The fixture exposes the Toxiproxy admin
// HTTP API for creating and removing proxies and toxics programmatically.
//
// Usage:
//
//	tp := toxiproxy.Start(ctx, t)
//	// tp.AdminURL is the Toxiproxy admin API base URL, e.g. "http://localhost:PORT"
package toxiproxy

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image     = "ghcr.io/shopify/toxiproxy:2.7.0"
	adminPort = "8474/tcp"
)

// Toxiproxy holds connection info for a running Toxiproxy container.
type Toxiproxy struct {
	// AdminURL is the base URL for the Toxiproxy admin API,
	// e.g. "http://localhost:PORT". Use it to create proxies and inject toxics.
	AdminURL string

	container testcontainers.Container
}

// Start spins up a fresh Toxiproxy container and registers t.Cleanup to
// terminate it. Skips the test cleanly if Docker is unavailable.
func Start(ctx context.Context, t *testing.T) *Toxiproxy {
	t.Helper()
	requireDocker(t)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{adminPort},
			WaitingFor:   wait.ForHTTP("/proxies").WithPort(adminPort),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("toxiproxy: start container: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("toxiproxy: cleanup: terminate: %v", err)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("toxiproxy: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, adminPort)
	if err != nil {
		t.Fatalf("toxiproxy: get port: %v", err)
	}

	return &Toxiproxy{
		AdminURL:  fmt.Sprintf("http://%s:%d", host, mappedPort.Num()),
		container: c,
	}
}

// CheckReachable verifies the Toxiproxy admin API is responding.
func (tp *Toxiproxy) CheckReachable() error {
	resp, err := http.Get(tp.AdminURL + "/proxies") //nolint:noctx
	if err != nil {
		return fmt.Errorf("toxiproxy: http check: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("toxiproxy: http check: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}
