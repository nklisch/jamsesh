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
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/containerlog"
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

	// ContainerFiles mounts host files into the container at fixed paths.
	// Use for _FILE secrets (e.g. JAMSESH_DB_DSN_FILE=/run/secrets/db_dsn).
	// Empty slice = no mounts, matching current default behavior.
	ContainerFiles []testcontainers.ContainerFile
}

// Portal holds connection info for a running portal container.
type Portal struct {
	// URL is the portal's base URL, e.g. "http://localhost:PORT".
	// The portal binds on :8443 inside the container; Testcontainers maps that
	// to a random host port exposed here.
	URL string

	container testcontainers.Container
}

// ContainerName returns the Docker container name without the leading slash,
// e.g. "tc-jamsesh-portal-abc123". Returns an empty string if the name cannot
// be retrieved. The name is stable for the lifetime of the test.
//
// Callers that need to interact with the container directly (e.g. via
// `docker pause`) should use this name.
func (p *Portal) ContainerName(ctx context.Context) string {
	name, err := p.container.Name(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(name, "/")
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

	// Defense in depth against accidentally hitting real github.com. A test
	// that wants GitHub OAuth must point JAMSESH_OAUTH_GITHUB_BASE_URL at a
	// stub (typically the WireMock fixture). Tests that don't need OAuth at
	// all leave both fields zero — the portal handles unconfigured GitHub
	// OAuth gracefully by returning 503 oauth.provider_not_configured on the
	// relevant endpoints. Refusing to start surfaces this decision at test-
	// design time instead of at first network call.
	if opts.OAuthGitHubClientID != "" && opts.OAuthBaseURL == "" {
		t.Fatalf("portal: OAuthGitHubClientID is set but OAuthBaseURL is empty; " +
			"configure OAuthBaseURL to point at WireMock or leave " +
			`OAuthGitHubClientID="" to disable GitHub OAuth in the portal entirely`)
	}

	env := buildEnv(opts)

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{containerPort},
			Env:          env,
			Files:        opts.ContainerFiles,
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
		containerlog.DumpAndTerminate(ctx, t, c, "portal")
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

	// Only inject GitHub OAuth credentials when the caller has explicitly
	// supplied a client ID. The portal treats missing CLIENT_ID/SECRET as
	// "github provider not configured" and returns 503 from any /api/auth/
	// oauth/* endpoint touching it. The defense-in-depth check in Start
	// already refuses if CLIENT_ID is non-empty without OAuthBaseURL, so a
	// non-empty CLIENT_ID here always means the caller has also pointed
	// OAuthBaseURL at a stub.
	if opts.OAuthGitHubClientID != "" {
		env["JAMSESH_OAUTH_GITHUB_CLIENT_ID"] = opts.OAuthGitHubClientID
		clientSecret := opts.OAuthGitHubClientSecret
		if clientSecret == "" {
			clientSecret = "test-secret"
		}
		env["JAMSESH_OAUTH_GITHUB_CLIENT_SECRET"] = clientSecret
	}

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
