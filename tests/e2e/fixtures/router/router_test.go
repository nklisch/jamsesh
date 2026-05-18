package router_test

import (
	"context"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/router"
)

// TestRouterProxy proves the router performs a real reverse-proxy round-trip:
// an nginx:alpine stub backend is started, the router is pointed at it via the
// Docker bridge network IP, and an HTTP GET through the router's host-mapped
// URL returns nginx's default welcome page.
func TestRouterProxy(t *testing.T) {
	requireDocker(t)
	requireRouterImage(t)

	ctx := context.Background()

	// Start an nginx stub backend.
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nginx:alpine",
			ExposedPorts: []string{"80/tcp"},
			WaitingFor:   wait.ForHTTP("/").WithPort("80/tcp"),
		},
		Started: true,
	}
	nginx, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("nginx start: %v", err)
	}
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(nginx)
	})

	// Use the container bridge IP so the router (also on the bridge) can reach
	// the nginx backend without going through the host.
	nginxIP, err := nginx.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("nginx IP: %v", err)
	}

	// Start the router pointed at nginx (container-side address).
	r := router.Start(ctx, t, router.Options{
		Backends: []string{nginxIP + ":80"},
	})

	// Hit the router URL; assert nginx's default page came back.
	resp, err := http.Get(r.URL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET router: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200; body=%q", resp.StatusCode, body)
	}
	// nginx default page contains "Welcome to nginx".
	if !strings.Contains(string(body), "Welcome to nginx") {
		t.Errorf("body did not look like nginx default page: %q", body)
	}
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requireRouterImage skips t with an actionable message if the router e2e
// image has not been built yet.
func requireRouterImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", "jamsesh/router:e2e").Run(); err != nil {
		t.Skip("router e2e image \"jamsesh/router:e2e\" not present — run `make test-router-image` first")
	}
}
