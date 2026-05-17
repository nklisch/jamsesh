// Package portal provides a Testcontainers-Go fixture for the jamsesh portal.
//
// The fixture starts the portal binary inside the jamsesh/portal:e2e Docker
// image (built by `make test-portal-image`) and waits until /healthz returns
// 200. Each invocation creates a fresh container so tests are fully isolated.
//
// If the image is absent the test is skipped with a clear message — no Docker
// backtrace. See requirePortalImage.
//
// Usage:
//
//	p := portal.Start(ctx, t, portal.Options{
//	    DBDriver:     "postgres",
//	    DBDSN:        pg.DSN,
//	    EmailFrom:    "noreply@example.com",
//	    SMTPHost:     mh.SMTPHost,
//	    SMTPPort:     mh.SMTPPort,
//	    OAuthBaseURL: wm.URL,
//	})
//	// p.URL is the portal's base URL, e.g. "http://localhost:PORT"
package portal

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image         = "jamsesh/portal:e2e"
	containerPort = "8443/tcp"
)

// Options configures a portal container. Every field maps directly to a
// JAMSESH_* environment variable documented in internal/portal/config/config.go.
type Options struct {
	// DBDriver is the database driver: "postgres" or "sqlite".
	// Default: "sqlite"
	DBDriver string

	// DBDSN is the data-source name for the chosen driver.
	// For SQLite in-process tests use ":memory:".
	// For Postgres use the postgres:// DSN from the postgres fixture.
	DBDSN string

	// EmailFrom is the envelope sender address. REQUIRED — the portal binary
	// calls senders.New() unconditionally at startup and hard-fails if empty.
	// Use "noreply@example.com" in tests.
	EmailFrom string

	// SMTPHost and SMTPPort configure the SMTP delivery backend.
	// Point these at the MailHog fixture when running the full stack.
	SMTPHost string
	SMTPPort int

	// OAuthBaseURL overrides the GitHub OAuth + API base URL.
	// Point this at the WireMock fixture to stub GitHub OAuth.
	// If empty, GitHub OAuth is effectively unconfigured (fine for /healthz).
	OAuthBaseURL string

	// OAuthGitHubClientID and OAuthGitHubClientSecret are the GitHub OAuth
	// application credentials. Non-empty values are required for any flow that
	// touches GitHub OAuth, but are not validated at startup.
	OAuthGitHubClientID     string
	OAuthGitHubClientSecret string

	// ExtraEnv passes additional JAMSESH_* env vars to the container.
	// Keys must be the full env-var name, e.g. "JAMSESH_LOG_LEVEL".
	ExtraEnv map[string]string
}

// Portal holds connection info for a running portal container.
type Portal struct {
	// URL is the portal's base URL, e.g. "http://localhost:PORT".
	// The portal binds on :8443 inside the container; Testcontainers maps that
	// to a random host port exposed here.
	URL string

	container testcontainers.Container
}

// Start spins up a fresh portal container with the given configuration,
// waits until /healthz returns 200, and registers t.Cleanup to terminate it.
//
// If the jamsesh/portal:e2e image is not present the test is skipped — build
// it first with `make test-portal-image`.
func Start(ctx context.Context, t *testing.T, opts Options) *Portal {
	t.Helper()
	requireDocker(t)
	requirePortalImage(t)

	env := buildEnv(opts)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{containerPort},
			Env:          env,
			WaitingFor: wait.ForHTTP("/healthz").
				WithPort(containerPort).
				WithStatusCodeMatcher(func(code int) bool { return code == 200 }).
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("portal: start container: %v\n\nHint: if the portal crashed, check its logs with `docker logs <id>`", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("portal: cleanup: terminate: %v", err)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("portal: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, containerPort)
	if err != nil {
		t.Fatalf("portal: get port: %v", err)
	}

	return &Portal{
		URL:       fmt.Sprintf("http://%s:%d", host, mappedPort.Num()),
		container: c,
	}
}

// buildEnv converts Options into the env map the container needs.
func buildEnv(opts Options) map[string]string {
	driver := opts.DBDriver
	if driver == "" {
		driver = "sqlite"
	}
	dsn := opts.DBDSN
	if dsn == "" {
		dsn = ":memory:"
	}

	env := map[string]string{
		"JAMSESH_BIND":      ":8443",
		"JAMSESH_TLS_MODE":  "behind_proxy",
		"JAMSESH_DB_DRIVER": driver,
		"JAMSESH_DB_DSN":    dsn,
		"JAMSESH_EMAIL_FROM": opts.EmailFrom,
		// Use /tmp for git bare-repo storage so the portal can write repos
		// regardless of the container user (nobody:nogroup on debian-based images).
		"JAMSESH_STORAGE": "/tmp/jamsesh-repos",
	}

	if opts.SMTPHost != "" {
		env["JAMSESH_EMAIL_PROVIDER"] = "smtp"
		env["JAMSESH_EMAIL_SMTP_HOST"] = opts.SMTPHost
		env["JAMSESH_EMAIL_SMTP_TLS"] = "none"
		if opts.SMTPPort != 0 {
			env["JAMSESH_EMAIL_SMTP_PORT"] = strconv.Itoa(opts.SMTPPort)
		}
	}

	if opts.OAuthBaseURL != "" {
		env["JAMSESH_OAUTH_GITHUB_BASE_URL"] = opts.OAuthBaseURL
	}

	clientID := opts.OAuthGitHubClientID
	if clientID == "" {
		clientID = "test-client"
	}
	clientSecret := opts.OAuthGitHubClientSecret
	if clientSecret == "" {
		clientSecret = "test-secret"
	}
	env["JAMSESH_OAUTH_GITHUB_CLIENT_ID"] = clientID
	env["JAMSESH_OAUTH_GITHUB_CLIENT_SECRET"] = clientSecret

	for k, v := range opts.ExtraEnv {
		env[k] = v
	}
	return env
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requirePortalImage skips t with an actionable message if the portal e2e
// image has not been built yet. This produces a clear skip rather than an
// opaque Docker error.
func requirePortalImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", image)
	}
}
