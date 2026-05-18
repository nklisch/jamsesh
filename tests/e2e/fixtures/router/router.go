// Package router provides a Testcontainers-Go fixture for jamsesh-router,
// the consistent-hash reverse proxy used in clustered mode.
//
// The fixture starts jamsesh-router inside the jamsesh/router:e2e Docker image
// (built by `make test-router-image`) configured in static-discovery mode and
// waits until /metrics returns 200. Each invocation creates a fresh container
// so tests are fully isolated.
//
// If the image is absent the test is skipped with a clear message — no Docker
// backtrace. See requireRouterImage.
//
// Usage:
//
//	r := router.Start(ctx, t, router.Options{
//	    Backends: []string{"10.0.0.1:8443", "10.0.0.2:8443"},
//	})
//	// r.URL is the router's host-side base URL, e.g. "http://127.0.0.1:PORT"
//	// r.ContainerURL is the bridge-network URL for container-to-container use
package router

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/containerlog"
)

const (
	image         = "jamsesh/router:e2e"
	containerPort = "8080/tcp"
)

// Options configures a router container. Fields map to the env-var surface
// documented in cmd/jamsesh-router/main.go printUsage.
//
// Note: HintCacheTTL is YAML-only in the router config (v1) and has no env-var
// binding, so it cannot be set via this fixture. Use the default (60s) or mount
// a config file via ExtraEnv / a future Options field if shorter TTLs are
// needed in test scenarios.
type Options struct {
	// Backends is the list of upstream portal pod addresses (host:port) the
	// router will reverse-proxy to. Used in static discovery mode. At least
	// one backend is required.
	//
	// When the backends are other containers on the Docker bridge network, pass
	// their ContainerIP:port here so the router (also on the bridge) can reach
	// them without host-port mapping.
	Backends []string
}

// Router holds connection info for a running jamsesh-router container.
type Router struct {
	// URL is the router's host-side base URL, e.g. "http://127.0.0.1:PORT".
	// Use this from the test process (outside Docker).
	URL string

	// ContainerURL is the router's bridge-network base URL,
	// e.g. "http://172.17.0.5:8080". Use this when another Docker container
	// (e.g. a client container) needs to reach the router — host-mapped ports
	// are not reachable from inside Docker.
	ContainerURL string

	container testcontainers.Container
}

// Start spins up a fresh jamsesh-router container with the given options,
// waits until /metrics returns 200, and registers t.Cleanup to terminate it.
//
// Skips the test cleanly if Docker is unavailable or the jamsesh/router:e2e
// image has not been built yet. Build it first with `make test-router-image`.
func Start(ctx context.Context, t *testing.T, opts Options) *Router {
	t.Helper()
	requireDocker(t)
	requireRouterImage(t)

	env := map[string]string{
		"JAMSESH_ROUTER_BIND":             ":8080",
		"JAMSESH_ROUTER_DISCOVERY_MODE":   "static",
		"JAMSESH_ROUTER_STATIC_PODS":      strings.Join(opts.Backends, ","),
		"JAMSESH_ROUTER_SHUTDOWN_GRACE_S": "5",
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{containerPort},
			Env:          env,
			WaitingFor: wait.ForHTTP("/metrics").
				WithPort(containerPort).
				WithStatusCodeMatcher(func(code int) bool { return code == 200 }).
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("router: start container: %v\n\nHint: if the router crashed, check its logs with `docker logs <id>`", err)
	}

	t.Cleanup(func() {
		containerlog.DumpAndTerminate(ctx, t, c, "router")
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("router: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, containerPort)
	if err != nil {
		t.Fatalf("router: get port: %v", err)
	}

	containerIP, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("router: get container IP: %v", err)
	}

	return &Router{
		URL:          fmt.Sprintf("http://%s:%d", host, mappedPort.Num()),
		ContainerURL: fmt.Sprintf("http://%s:8080", containerIP),
		container:    c,
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
// image has not been built yet. This produces a clear skip rather than an
// opaque Docker error.
func requireRouterImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("router e2e image %q not present — run `make test-router-image` first", image)
	}
}
