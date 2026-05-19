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
	"net/http"
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

	// BackendReadyzURLs, if set, are host-side base URLs (e.g. "http://127.0.0.1:PORT")
	// for each backend portal. Start polls each URL's /readyz endpoint and waits
	// until all return HTTP 200 before creating the router container.
	//
	// This prevents a race where the router's static discoverer fires its first
	// /readyz probe before portals have completed their Postgres-ping + os.Stat
	// readiness checks. Without this poll, the discovery goroutine may publish([])
	// and evict the pre-seeded ring, causing 503 on the first requests.
	//
	// Use Backends for the router's static-discovery config (container-side IPs)
	// and BackendReadyzURLs for the host-side poll (reachable from the test process).
	// Leave nil to skip the poll (e.g. when backends are not portal containers).
	BackendReadyzURLs []string
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

	// Wait for all backend portals to pass /readyz before starting the router.
	// This eliminates the race where the router's discovery goroutine fires its
	// first /readyz probe before portals have finished their Postgres-ping +
	// os.Stat readiness checks. Without this wait, that probe returns zero
	// healthy pods and publish([]) evicts the pre-seeded ring, causing 503.
	//
	// Note: the discoverer also no longer probes immediately on startup (it
	// waits one full interval first), but this fixture-side poll is a second
	// layer of defence: it ensures portals are genuinely ready before the
	// router container even starts, so both the first tick and any early
	// requests are safe.
	if len(opts.BackendReadyzURLs) > 0 {
		waitForBackendsReadyz(ctx, t, opts.BackendReadyzURLs)
	}

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

// waitForBackendsReadyz polls each base URL's /readyz endpoint until all
// return HTTP 200 or the context is cancelled. It logs progress so test output
// is actionable when a portal is slow to become ready.
//
// Each URL should be a host-side base URL (e.g. "http://127.0.0.1:PORT") —
// the /readyz path is appended automatically.
// readyzWaitTimeout bounds the readyz-poll so a misconfigured portal surfaces
// as a fast, actionable failure rather than a 10-minute test-package timeout.
const readyzWaitTimeout = 60 * time.Second

func waitForBackendsReadyz(ctx context.Context, t *testing.T, baseURLs []string) {
	t.Helper()

	pollCtx, cancel := context.WithTimeout(ctx, readyzWaitTimeout)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}
	pending := make([]bool, len(baseURLs))
	for i := range pending {
		pending[i] = true // all start as not-yet-ready
	}

	t.Logf("router fixture: waiting for %d backend portal(s) to pass /readyz (timeout %s)", len(baseURLs), readyzWaitTimeout)

	var lastStatuses []string
	for {
		allReady := true
		lastStatuses = lastStatuses[:0]
		for i, ready := range pending {
			if !ready {
				continue
			}
			url := baseURLs[i] + "/readyz"
			req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("router fixture: waitForBackendsReadyz: build request for %s: %v", url, err)
			}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					pending[i] = false // this backend is ready
					t.Logf("router fixture: backend %s is ready", baseURLs[i])
					continue
				}
				lastStatuses = append(lastStatuses, fmt.Sprintf("%s=HTTP %d", baseURLs[i], resp.StatusCode))
			} else {
				lastStatuses = append(lastStatuses, fmt.Sprintf("%s=err:%v", baseURLs[i], err))
			}
			allReady = false
		}
		if allReady {
			return
		}
		select {
		case <-pollCtx.Done():
			t.Fatalf("router fixture: backends did not pass /readyz within %s; last statuses: %s",
				readyzWaitTimeout, strings.Join(lastStatuses, ", "))
		case <-time.After(200 * time.Millisecond):
		}
	}
}
