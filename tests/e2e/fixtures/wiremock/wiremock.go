// Package wiremock provides a Testcontainers-Go fixture for WireMock.
//
// WireMock is an HTTP stub server. The fixture mounts local mapping-JSON files
// into the container at /home/wiremock/mappings/ so stubs are available
// immediately on startup.
//
// Usage:
//
//	wm := wiremock.Start(ctx, t, wiremock.Mappings{
//	    "github": "fixtures/wiremock/mappings/github.json",
//	})
//	// wm.URL is the base URL for stubbed requests, e.g. "http://localhost:PORT"
package wiremock

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image    = "wiremock/wiremock:3.5.4"
	httpPort = "8080/tcp"
)

// Mappings maps a short name to a host-side JSON mapping file path.
// Each file is mounted into /home/wiremock/mappings/<name>.json inside the
// container. Paths may be absolute or relative to the test's working directory.
type Mappings map[string]string

// WireMock holds connection info for a running WireMock container.
type WireMock struct {
	// URL is the base URL for issuing requests to stubbed endpoints using the
	// host-side mapped port, e.g. "http://localhost:PORT". Use this from the
	// test process.
	URL string

	// ContainerURL is the base URL using the Docker bridge IP and internal
	// port (8080). Use this when another Docker container (e.g. the portal
	// fixture) needs to reach WireMock — from inside Docker the host-mapped
	// port is not reachable but the bridge IP is.
	ContainerURL string

	container testcontainers.Container
}

// Start spins up a fresh WireMock container with the given stub mappings mounted
// and registers t.Cleanup to terminate it. Skips cleanly if Docker is unavailable.
func Start(ctx context.Context, t *testing.T, mappings Mappings) *WireMock {
	t.Helper()
	requireDocker(t)

	files := make([]testcontainers.ContainerFile, 0, len(mappings))
	for name, hostPath := range mappings {
		// Resolve relative paths so Testcontainers finds them correctly.
		abs, err := filepath.Abs(hostPath)
		if err != nil {
			t.Fatalf("wiremock: resolve mapping path %q: %v", hostPath, err)
		}
		files = append(files, testcontainers.ContainerFile{
			HostFilePath:      abs,
			ContainerFilePath: "/home/wiremock/mappings/" + name + ".json",
			FileMode:          0o644,
		})
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{httpPort},
			Files:        files,
			WaitingFor:   wait.ForHTTP("/__admin/mappings").WithPort(httpPort),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("wiremock: start container: %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("wiremock: cleanup: terminate: %v", err)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("wiremock: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, httpPort)
	if err != nil {
		t.Fatalf("wiremock: get port: %v", err)
	}

	containerIP, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("wiremock: get container IP: %v", err)
	}

	return &WireMock{
		URL:          fmt.Sprintf("http://%s:%d", host, mappedPort.Num()),
		ContainerURL: fmt.Sprintf("http://%s:8080", containerIP),
		container:    c,
	}
}

// CheckReachable verifies the WireMock admin API is responding.
func (w *WireMock) CheckReachable() error {
	resp, err := http.Get(w.URL + "/__admin/mappings") //nolint:noctx
	if err != nil {
		return fmt.Errorf("wiremock: http check: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wiremock: http check: unexpected status %d", resp.StatusCode)
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
