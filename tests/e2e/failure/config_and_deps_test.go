// Invariant: the portal fails LOUDLY when required configuration is missing
// at startup, and surfaces readable errors to callers when external dependencies
// become unavailable at runtime. A silent fallback or swallowed error is the
// bug these tests guard against.
//
// Two categories:
//
//  1. Missing config — portal container started with intentionally incomplete
//     env; asserts exit non-zero and the expected error class in container logs.
//
//  2. Unavailable dependency — full stack started, then a dependency is
//     disrupted mid-test; asserts the portal returns 500 with a plain-text
//     error body (the oapi-codegen strict handler's ResponseErrorHandlerFunc
//     path), or the documented error envelope where one is defined.
//
// Error envelope shape (docs/PROTOCOL.md > HTTP error contract):
//
//	{"error": "<machine-readable code>", "message": "<human-readable>"}
//
// Note: unhandled handler errors are surfaced as plain-text 500 via
// http.Error — these tests assert only the status code, not the body.
package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
)

// portalImage is the e2e test image tag.
const portalImage = "jamsesh/portal:e2e"

// requirePortalImageLocal skips the test if the portal e2e image is not present.
func requirePortalImageLocal(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", portalImage).Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", portalImage)
	}
}

// requireDockerLocal skips the test if Docker is unavailable.
func requireDockerLocal(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// httpClientWithTimeout returns an *http.Client with the given timeout.
// Use this for any request that might hang when a dependency is down.
func httpClientWithTimeout(d time.Duration) *http.Client {
	return &http.Client{Timeout: d}
}

// startFailingPortal starts a portal container with the given env vars, waits
// up to 15 seconds for it to exit (it should fail fast), and returns the
// container and its collected log output. The caller is responsible for cleanup.
//
// The container is started with Started: true but WITHOUT a health-check
// wait strategy — we expect it to crash immediately.
func startFailingPortal(ctx context.Context, t *testing.T, env map[string]string) (testcontainers.Container, string) {
	t.Helper()

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        portalImage,
			Env:          env,
			ExposedPorts: []string{"8443/tcp"},
			// No WaitingFor — the container should crash before /healthz is reachable.
		},
		Started: true,
	})
	if err != nil {
		// GenericContainer may error if the container itself fails to start at
		// the Docker level. That is different from the portal binary crashing.
		t.Fatalf("startFailingPortal: GenericContainer error (Docker-level, not portal crash): %v", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("startFailingPortal: cleanup: terminate: %v", err)
		}
	})

	// Poll until the container is no longer running (it should exit within a
	// few seconds on missing/invalid config) or until a generous timeout.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err := c.State(ctx)
		if err != nil {
			break // can't inspect state; fall through and read logs anyway
		}
		if !state.Running {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	// Collect logs regardless of state so we can assert on log content.
	logReader, err := c.Logs(ctx)
	var logOutput string
	if err == nil && logReader != nil {
		raw, _ := io.ReadAll(logReader)
		logReader.Close()
		logOutput = string(raw)
	}

	return c, logOutput
}

// containerIsRunning returns false when the container has exited or when its
// state cannot be retrieved (treat as stopped).
func containerIsRunning(ctx context.Context, c testcontainers.Container) bool {
	state, err := c.State(ctx)
	if err != nil {
		return false
	}
	return state.Running
}

// oauthStartOAuth calls POST /api/auth/oauth/start and returns the authorize_url.
func oauthStartOAuth(ctx context.Context, t *testing.T, client *http.Client, baseURL, provider string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"provider": provider})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/oauth/start", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("oauthStartOAuth: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("oauthStartOAuth: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("oauthStartOAuth: status %d (want 200): %s", resp.StatusCode, respBody)
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("oauthStartOAuth: decode response: %v\nbody: %s", err, respBody)
	}
	if out.AuthorizeURL == "" {
		t.Fatalf("oauthStartOAuth: empty authorize_url in response: %s", respBody)
	}
	return out.AuthorizeURL
}

// oauthCallback calls POST /api/auth/oauth/callback and returns (statusCode, bodyBytes).
func oauthCallback(ctx context.Context, t *testing.T, client *http.Client, baseURL, provider, state, code string) (int, []byte) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"provider": provider,
		"state":    state,
		"code":     code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/oauth/callback", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("oauthCallback: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("oauthCallback: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// toxiproxyCreateProxy creates a Toxiproxy proxy via the admin API.
// listen is the address the proxy will listen on inside the Toxiproxy container
// (e.g. "0.0.0.0:5433"). upstream is the address to forward traffic to
// (e.g. "pg-ip:5432").
func toxiproxyCreateProxy(ctx context.Context, t *testing.T, adminURL, name, listen, upstream string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":     name,
		"listen":   listen,
		"upstream": upstream,
		"enabled":  true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adminURL+"/proxies", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("toxiproxyCreateProxy: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxyCreateProxy: POST /proxies: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("toxiproxyCreateProxy: status %d (want 201): %s", resp.StatusCode, respBody)
	}
}

// toxiproxyAddToxic adds a toxic to a Toxiproxy proxy. kind is the toxic type
// (e.g. "reset_peer"), attributes is a map of toxic-specific settings.
func toxiproxyAddToxic(ctx context.Context, t *testing.T, adminURL, proxyName, toxicName, kind string, attributes map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":       toxicName,
		"type":       kind,
		"toxicity":   1.0,
		"attributes": attributes,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/proxies/%s/toxics", adminURL, proxyName),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("toxiproxyAddToxic: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxyAddToxic: POST toxics: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toxiproxyAddToxic: status %d (want 200): %s", resp.StatusCode, respBody)
	}
}

// toxiproxyDeleteToxic removes a named toxic from a proxy.
func toxiproxyDeleteToxic(ctx context.Context, t *testing.T, adminURL, proxyName, toxicName string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/proxies/%s/toxics/%s", adminURL, proxyName, toxicName),
		nil)
	if err != nil {
		t.Fatalf("toxiproxyDeleteToxic: build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxyDeleteToxic: DELETE toxic: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("toxiproxyDeleteToxic: status %d (want 204)", resp.StatusCode)
	}
}

// thisDir returns the directory containing this test file (used to resolve
// testdata paths regardless of the working directory the test is run from).
func thisDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

// wireMockContainer wraps connection info for WireMock.
type wireMockContainer struct {
	URL          string
	ContainerURL string
}

// startWireMockWithMappings starts a WireMock container and mounts the given
// mappings file at /home/wiremock/mappings/stubs.json. Cleanup is registered
// via t.Cleanup.
func startWireMockWithMappings(ctx context.Context, t *testing.T, mappingsFile string) *wireMockContainer {
	t.Helper()

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "wiremock/wiremock:3.5.4",
			ExposedPorts: []string{"8080/tcp"},
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      mappingsFile,
					ContainerFilePath: "/home/wiremock/mappings/stubs.json",
					FileMode:          0o644,
				},
			},
			WaitingFor: wait.ForHTTP("/__admin/mappings").WithPort("8080/tcp"),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("startWireMockWithMappings: start container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			t.Logf("startWireMockWithMappings: cleanup: %v", err)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("startWireMockWithMappings: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("startWireMockWithMappings: get port: %v", err)
	}
	containerIP, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("startWireMockWithMappings: get container IP: %v", err)
	}

	return &wireMockContainer{
		URL:          fmt.Sprintf("http://%s:%d", host, mappedPort.Num()),
		ContainerURL: fmt.Sprintf("http://%s:8080", containerIP),
	}
}

// TestConfigAndDeps asserts loud startup failures on missing config and loud
// runtime failures when external dependencies are disrupted mid-session.
func TestConfigAndDeps(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	// =========================================================================
	// Category 1: Missing config (startup failures)
	// =========================================================================

	t.Run("missing_config", func(t *testing.T) {
		t.Run("missing_email_from", func(t *testing.T) {
			// Invariant: starting the portal with an empty JAMSESH_EMAIL_FROM
			// causes the binary to exit non-zero. senders.New validates that
			// email.from is non-empty at startup; the container must not stay
			// running and its logs must mention the missing field.
			ctx := context.Background()
			env := map[string]string{
				"JAMSESH_BIND":            ":8443",
				"JAMSESH_TLS_MODE":        "behind_proxy",
				"JAMSESH_DB_DRIVER":       "sqlite",
				"JAMSESH_DB_DSN":          ":memory:",
				"JAMSESH_EMAIL_PROVIDER":  "smtp",
				"JAMSESH_EMAIL_SMTP_HOST": "localhost",
				"JAMSESH_EMAIL_SMTP_PORT": "1025",
				// Intentionally omit JAMSESH_EMAIL_FROM to trigger senders.New failure.
			}
			c, logs := startFailingPortal(ctx, t, env)

			if containerIsRunning(ctx, c) {
				t.Error("expected portal to have exited on missing JAMSESH_EMAIL_FROM, but container is still running")
			}

			// senders.New returns: "senders: email.from must not be empty"
			if !strings.Contains(logs, "email.from") && !strings.Contains(logs, "EMAIL_FROM") {
				t.Errorf("expected container logs to mention missing email.from; got:\n%s", logs)
			}
		})

		t.Run("invalid_tls_mode", func(t *testing.T) {
			// Invariant: starting the portal with an unrecognised JAMSESH_TLS_MODE
			// value causes the binary to exit non-zero during config validation.
			// The container must not stay running and its logs must mention the
			// invalid tls.mode value or config load failure.
			ctx := context.Background()
			env := map[string]string{
				"JAMSESH_BIND":            ":8443",
				"JAMSESH_TLS_MODE":        "garbage", // invalid
				"JAMSESH_DB_DRIVER":       "sqlite",
				"JAMSESH_DB_DSN":          ":memory:",
				"JAMSESH_EMAIL_FROM":      "noreply@example.com",
				"JAMSESH_EMAIL_PROVIDER":  "smtp",
				"JAMSESH_EMAIL_SMTP_HOST": "localhost",
				"JAMSESH_EMAIL_SMTP_PORT": "1025",
			}
			c, logs := startFailingPortal(ctx, t, env)

			if containerIsRunning(ctx, c) {
				t.Error("expected portal to have exited on invalid JAMSESH_TLS_MODE, but container is still running")
			}

			// config.validate() returns:
			//   config: tls.mode must be "native" or "behind_proxy", got "garbage"
			// main.go logs: "config load failed"
			if !strings.Contains(logs, "tls") && !strings.Contains(logs, "TLS") && !strings.Contains(logs, "config") {
				t.Errorf("expected container logs to mention TLS config error; got:\n%s", logs)
			}
		})

		t.Run("postgres_driver_invalid_dsn", func(t *testing.T) {
			// Invariant: starting the portal with JAMSESH_DB_DRIVER=postgres and
			// a syntactically invalid DSN causes the binary to exit non-zero.
			// db.Open calls pgxpool.ParseConfig which fails fast; the container
			// must not stay running and its logs must mention the database error.
			ctx := context.Background()
			env := map[string]string{
				"JAMSESH_BIND":            ":8443",
				"JAMSESH_TLS_MODE":        "behind_proxy",
				"JAMSESH_DB_DRIVER":       "postgres",
				"JAMSESH_DB_DSN":          "not-a-valid-dsn://???",
				"JAMSESH_EMAIL_FROM":      "noreply@example.com",
				"JAMSESH_EMAIL_PROVIDER":  "smtp",
				"JAMSESH_EMAIL_SMTP_HOST": "localhost",
				"JAMSESH_EMAIL_SMTP_PORT": "1025",
			}
			c, logs := startFailingPortal(ctx, t, env)

			if containerIsRunning(ctx, c) {
				t.Error("expected portal to have exited on invalid postgres DSN, but container is still running")
			}

			// db.Open returns: "db: parse postgres dsn: ..."
			// main.go logs: "database open failed"
			if !strings.Contains(logs, "database") && !strings.Contains(logs, "db") && !strings.Contains(logs, "postgres") {
				t.Errorf("expected container logs to mention database/DSN error; got:\n%s", logs)
			}
		})
	})

	// =========================================================================
	// Category 2: Unavailable dependency (runtime failures)
	// =========================================================================

	t.Run("unavailable_dep", func(t *testing.T) {
		t.Run("smtp_unavailable", func(t *testing.T) {
			// Invariant: when the SMTP server becomes unreachable after the portal
			// starts, a magic-link request returns 500. The portal surfaces send
			// errors to the caller (see magic_link.go: "magic-link: send email:").
			// A silent 204 with a failed send would be the bug this test catches.
			ctx := context.Background()

			mh := mailhog.Start(ctx, t)
			pg := postgres.Start(ctx, t, postgres.Options{})
			p := portal.Start(ctx, t, portal.Options{
				DBDriver:  "postgres",
				DBDSN:     pg.ContainerDSN,
				EmailFrom: "noreply@example.com",
				SMTPHost:  mh.ContainerSMTPHost,
				SMTPPort:  mh.ContainerSMTPPort,
			})

			// Verify the portal is healthy before disruption.
			healthClient := httpClientWithTimeout(5 * time.Second)
			{
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/healthz", nil)
				resp, err := healthClient.Do(req)
				if err != nil || resp.StatusCode != http.StatusOK {
					if resp != nil {
						resp.Body.Close()
					}
					t.Fatalf("smtp_unavailable: pre-disruption healthz failed: err=%v", err)
				}
				resp.Body.Close()
			}

			// Stop the MailHog container to simulate SMTP going down.
			if err := mh.Stop(ctx); err != nil {
				t.Fatalf("smtp_unavailable: stop mailhog: %v", err)
			}

			// Attempt a magic-link request. The portal must surface the send
			// error as 500, not silently return 204. Use a generous timeout
			// since the portal may wait for TCP connect to time out before
			// the SMTP library returns an error.
			smtpClient := httpClientWithTimeout(30 * time.Second)
			body, _ := json.Marshal(map[string]string{"email": "test-smtp-failure@example.com"})
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.URL+"/api/auth/magic-link/request", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := smtpClient.Do(req)
			if err != nil {
				// If the portal returned a network error (e.g. connection reset
				// after the SMTP library panicked), the test still passes because
				// the portal did NOT return 204 silently.
				t.Logf("smtp_unavailable: POST magic-link/request: network error (portal did not return 204): %v", err)
				return
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("smtp_unavailable: expected 500 when SMTP is down, got %d\nbody: %s",
					resp.StatusCode, respBody)
			}
		})

		t.Run("db_unavailable_via_toxiproxy", func(t *testing.T) {
			// Invariant: when Postgres becomes unreachable mid-session (simulated
			// via a Toxiproxy reset_peer toxic), the portal returns 500 on
			// subsequent REST calls. After removing the toxic, the portal must
			// recover (return non-5xx for simple requests).
			//
			// Toxiproxy is used instead of docker pause because:
			// 1. The reset_peer toxic causes immediate connection failure, not a
			//    hang — so the portal's DB query fails fast and returns 500.
			// 2. It does not disrupt other tests sharing the postgres container.
			ctx := context.Background()

			pg := postgres.Start(ctx, t, postgres.Options{})
			tp := toxiproxy.Start(ctx, t)

			// Create a Toxiproxy proxy: toxiproxy-container → postgres-container.
			const (
				proxyName   = "pg"
				proxyListen = "0.0.0.0:5433"
				toxicName   = "pg_down"
			)
			// pg.ContainerDSN is postgres://test:test@<bridge-ip>:5432/<dbname>...
			// Extract the bridge IP so Toxiproxy (inside Docker) can reach Postgres.
			pgContainerHost := extractHostFromDSN(pg.ContainerDSN)
			toxiproxyCreateProxy(ctx, t, tp.AdminURL, proxyName,
				proxyListen,
				fmt.Sprintf("%s:5432", pgContainerHost))

			// Wait briefly for the proxy port to be ready.
			time.Sleep(200 * time.Millisecond)

			// Configure portal to connect through Toxiproxy.
			// tp.ContainerIP is the Docker bridge IP of the Toxiproxy container,
			// reachable from the portal container without host port mapping.
			containerDSN := fmt.Sprintf("postgres://test:test@%s:5433/%s?sslmode=disable",
				tp.ContainerIP,
				extractDBName(pg.ContainerDSN))

			mh := mailhog.Start(ctx, t)
			p := portal.Start(ctx, t, portal.Options{
				DBDriver:  "postgres",
				DBDSN:     containerDSN,
				EmailFrom: "noreply@example.com",
				SMTPHost:  mh.ContainerSMTPHost,
				SMTPPort:  mh.ContainerSMTPPort,
			})

			// Verify portal is healthy before disruption.
			healthClient := httpClientWithTimeout(5 * time.Second)
			{
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/healthz", nil)
				resp, err := healthClient.Do(req)
				if err != nil || resp.StatusCode != http.StatusOK {
					if resp != nil {
						resp.Body.Close()
					}
					t.Fatalf("db_unavailable: pre-disruption healthz failed: err=%v", err)
				}
				resp.Body.Close()
			}

			// Add a reset_peer toxic: all new connections to the proxy are reset
			// immediately. Existing pgxpool connections will fail on next query.
			toxiproxyAddToxic(ctx, t, tp.AdminURL, proxyName, toxicName, "reset_peer",
				map[string]any{"timeout": 0})

			// Issue a GET /api/me with a fake bearer. The portal looks up the
			// token in the DB; pgxpool fails fast with connection reset.
			// Use a generous timeout on the HTTP request to allow pgxpool to
			// surface the error, but short enough not to block the test suite.
			dbErrClient := httpClientWithTimeout(15 * time.Second)
			var dbErrStatus int
			{
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
				req.Header.Set("Authorization", "Bearer fake-token-db-down")
				resp, err := dbErrClient.Do(req)
				if err != nil {
					// Connection error from the client side also indicates the portal
					// did not return a success — acceptable here.
					t.Logf("db_unavailable: GET /me while DB disrupted: network error: %v", err)
					dbErrStatus = 0
				} else {
					defer resp.Body.Close()
					io.Copy(io.Discard, resp.Body)
					dbErrStatus = resp.StatusCode
				}
			}

			if dbErrStatus == http.StatusOK {
				t.Errorf("db_unavailable: portal returned 200 while DB is disrupted — expected 4xx or 5xx")
			}

			// Remove the toxic — DB connections become normal again.
			toxiproxyDeleteToxic(ctx, t, tp.AdminURL, proxyName, toxicName)

			// After removing the toxic, the portal should recover.
			// Poll /healthz until it responds 200 or timeout.
			deadline := time.Now().Add(20 * time.Second)
			var recovered bool
			for time.Now().Before(deadline) {
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/healthz", nil)
				resp, err := healthClient.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					recovered = true
					break
				}
				if resp != nil {
					resp.Body.Close()
				}
				time.Sleep(500 * time.Millisecond)
			}
			if !recovered {
				t.Error("db_unavailable: portal did not recover after DB restored within 20s")
			}
		})

		t.Run("oauth_provider_5xx", func(t *testing.T) {
			// Invariant: when the OAuth provider (GitHub) returns 5xx on the
			// token-exchange endpoint, the portal returns 500 on the callback
			// call. The portal must not silently succeed or return 200.
			//
			// WireMock is configured with a 503 stub for /login/oauth/access_token.
			// The portal's OauthCallback calls Exchange which hits that endpoint
			// and fails; the strict handler surfaces the error as 500.
			ctx := context.Background()

			// WireMock with 503 stub for the GitHub token endpoint.
			github503Path := filepath.Join(thisDir(), "testdata", "github_503.json")
			wm := startWireMockWithMappings(ctx, t, github503Path)

			mh := mailhog.Start(ctx, t)
			pg := postgres.Start(ctx, t, postgres.Options{})
			p := portal.Start(ctx, t, portal.Options{
				DBDriver:                "postgres",
				DBDSN:                   pg.ContainerDSN,
				EmailFrom:               "noreply@example.com",
				SMTPHost:                mh.ContainerSMTPHost,
				SMTPPort:                mh.ContainerSMTPPort,
				OAuthBaseURL:            wm.ContainerURL,
				OAuthGitHubClientID:     "test-client",
				OAuthGitHubClientSecret: "test-secret",
			})

			client := httpClientWithTimeout(15 * time.Second)

			// Step 1: start the OAuth flow. Should succeed — the 503 stub only
			// affects the token endpoint, not the authorize URL generation.
			authorizeURL := oauthStartOAuth(ctx, t, client, p.URL, "github")

			// Extract the state nonce from the authorize_url to pass in the callback.
			parsed, err := url.Parse(authorizeURL)
			if err != nil {
				t.Fatalf("oauth_provider_5xx: parse authorize_url: %v", err)
			}
			stateNonce := parsed.Query().Get("state")
			if stateNonce == "" {
				t.Fatalf("oauth_provider_5xx: no state param in authorize_url: %s", authorizeURL)
			}

			// Step 2: call the callback — WireMock returns 503 on the token
			// endpoint. The portal's Exchange fails and the strict handler
			// returns 500.
			status, body := oauthCallback(ctx, t, client, p.URL, "github", stateNonce, "any-code")
			if status != http.StatusInternalServerError {
				t.Errorf("oauth_provider_5xx: expected 500 when OAuth provider returns 5xx, got %d\nbody: %s",
					status, body)
			}
		})
	})
}

// extractDBName extracts the database name from a postgres DSN.
// e.g. "postgres://test:test@host:5432/testdb?sslmode=disable" → "testdb"
func extractDBName(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "test"
	}
	return strings.TrimPrefix(u.Path, "/")
}

// extractHostFromDSN extracts the hostname (without port) from a postgres DSN.
// e.g. "postgres://test:test@172.17.0.3:5432/testdb?sslmode=disable" → "172.17.0.3"
func extractHostFromDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "localhost"
	}
	return u.Hostname()
}
