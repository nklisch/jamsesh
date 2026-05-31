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
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/containerlog"
)

const (
	image         = "jamsesh/portal:e2e"
	containerPort = "8443/tcp"

	// startupAttemptTimeout bounds how long each container-start attempt waits
	// for /healthz before being treated as a transient stall and retried.
	startupAttemptTimeout = 60 * time.Second
	// startupMaxAttempts is the number of container-start attempts before
	// giving up. Healthy boots succeed on the first; retries absorb transient
	// readiness stalls on a busy shared Docker host (the portal binary itself
	// boots in ~1s, so a stalled attempt is host contention, not a crash).
	startupMaxAttempts = 5
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

// Logs returns the combined stdout+stderr output of the portal container as a
// string. It reads the full log stream synchronously from the container runtime
// and is suitable for post-hoc inspection (e.g. grepping for a log phrase after
// the container has started healthy).
//
// For failure-mode log dumps use the containerlog.DumpAndTerminate helper wired
// by t.Cleanup in Start — Logs is for explicit, test-initiated captures.
func (p *Portal) Logs(ctx context.Context) (string, error) {
	rc, err := p.container.Logs(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ContainerIP returns the Docker bridge network IP of the portal container.
// Use this when another container (e.g. jamsesh-router) needs to reach the
// portal directly on the bridge network — host-mapped ports are not reachable
// from inside Docker.
func (p *Portal) ContainerIP(ctx context.Context) (string, error) {
	if p.container == nil {
		return "", fmt.Errorf("portal: ContainerIP: container is nil")
	}
	return p.container.ContainerIP(ctx)
}

// State returns the current container state as reported by the Docker daemon.
// Use this to inspect container lifecycle status (e.g. "running", "exited")
// without going through the Docker daemon's top-level inspect command.
func (p *Portal) State(ctx context.Context) (*container.State, error) {
	if p.container == nil {
		return nil, fmt.Errorf("portal: State: container is nil")
	}
	return p.container.State(ctx)
}

// SendSignal sends a Unix signal to PID 1 inside the container using BusyBox
// kill (available on alpine-based images). Use this to test graceful-shutdown
// behaviour (SIGTERM) without going through the Docker daemon's stop command.
//
// The kill is executed inside the container via exec, so it reaches the
// process that is PID 1 in the container's PID namespace — typically the
// portal binary itself when started without an init wrapper.
func (p *Portal) SendSignal(ctx context.Context, sig syscall.Signal) error {
	if p.container == nil {
		return fmt.Errorf("portal: SendSignal: container is nil")
	}
	code, _, err := p.container.Exec(ctx, []string{"kill", "-" + signalName(sig), "1"})
	if err != nil {
		return fmt.Errorf("portal: SendSignal(%v): exec: %w", sig, err)
	}
	if code != 0 {
		return fmt.Errorf("portal: SendSignal(%v): kill exited %d", sig, code)
	}
	return nil
}

// Exec runs an arbitrary command inside the portal container and returns the
// exit code and combined stdout+stderr output. Use this for test-side
// inspection (e.g. checking whether a directory exists on the container's
// filesystem) without modifying production code.
//
// The exit code is 0 on success. Non-zero exit codes are NOT treated as errors
// by this method — the caller decides whether a non-zero code is a failure.
//
// A non-nil error indicates a Docker API failure, not a command failure.
func (p *Portal) Exec(ctx context.Context, cmd []string) (exitCode int, output string, err error) {
	if p.container == nil {
		return -1, "", fmt.Errorf("portal: Exec: container is nil")
	}
	code, reader, execErr := p.container.Exec(ctx, cmd)
	if execErr != nil {
		return -1, "", fmt.Errorf("portal: Exec %v: %w", cmd, execErr)
	}
	if reader != nil {
		data, readErr := io.ReadAll(reader)
		if readErr == nil {
			output = string(data)
		}
	}
	return code, output, nil
}

// signalName maps common signals to their symbolic names as understood by
// BusyBox kill on alpine. Falls back to the numeric string for unknown signals.
func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGTERM:
		return "TERM"
	case syscall.SIGKILL:
		return "KILL"
	case syscall.SIGINT:
		return "INT"
	default:
		return fmt.Sprintf("%d", int(sig))
	}
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
				// Healthy boots are ~1s. This per-attempt ceiling is generous
				// headroom for a cold-start on a busy Docker host; if it is
				// exceeded the boot is treated as a transient stall and retried
				// below rather than failing the test outright.
				WithStartupTimeout(startupAttemptTimeout),
		},
		Started: true,
	}

	// Retry the start on readiness failure. On a shared/busy Docker host an
	// individual portal cold-start occasionally stalls past the readiness
	// deadline even though the binary is healthy (observed ~4 stalls per ~80
	// boots in the fuzz suites, each an isolated single-container event). These
	// stalls are transient, so a fresh container almost always comes up cleanly.
	// A successful first attempt is unaffected; only failures pay the retry cost.
	var c testcontainers.Container
	var err error
	for attempt := 1; attempt <= startupMaxAttempts; attempt++ {
		c, err = testcontainers.GenericContainer(ctx, req)
		if err == nil {
			break
		}
		// Best-effort terminate the half-started container before retrying so we
		// do not leak it (GenericContainer may return a non-nil container on a
		// readiness failure). Use a fresh, short cleanup context: the caller's
		// ctx may already be canceled (test deadline/cancel), which would make
		// Terminate(ctx) a no-op and leak the container during a long run.
		if c != nil {
			termCtx, cancelTerm := context.WithTimeout(context.Background(), 30*time.Second)
			_ = c.Terminate(termCtx)
			cancelTerm()
			c = nil
		}
		if attempt < startupMaxAttempts {
			t.Logf("portal: start container attempt %d/%d failed (%v) — retrying",
				attempt, startupMaxAttempts, err)
		}
	}
	if err != nil {
		t.Fatalf("portal: start container failed after %d attempts: %v\n\nHint: if the portal crashed, check its logs with `docker logs <id>`",
			startupMaxAttempts, err)
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
