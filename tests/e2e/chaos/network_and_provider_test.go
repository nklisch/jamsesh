// Invariant: the portal degrades gracefully under adverse network conditions.
// Chaos tests prove the difference between "looks fine on golden path" and
// "actually robust". Where a production invariant is absent, the test is
// skipped with a clear reference to the backlog item that tracks the fix.
//
// Active scenarios:
//
//   - network_jitter_db  — Toxiproxy injects 500ms latency between portal and
//     Postgres. Requests either succeed (elevated latency) or surface a clear
//     non-2xx status; no partial-state writes.
//
//   - oauth_provider_timeout — WireMock adds 30s delay to GitHub token
//     endpoint. The portal's 15s HTTP client timeout fires first (well
//     before WireMock's 30s response) and the callback returns a non-2xx
//     error within ~15s.
//
//   - ws_reconnect_drop — DEFERRED. Requires spa-websocket-reconnect-logic
//     (SPA-side reconnect) and wsclient.ConnectFromSeq (Go test helper).
package chaos_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
	"jamsesh/tests/e2e/fixtures/wiremock"
)

// TestNetworkAndProvider is the top-level chaos test. Each sub-test brings up
// its own full stack so chaos in one scenario cannot bleed into another.
func TestNetworkAndProvider(t *testing.T) {
	t.Run("network_jitter_db", testNetworkJitterDB)
	t.Run("oauth_provider_timeout", testOAuthProviderTimeout)
	t.Run("ws_reconnect_drop", testWSReconnectDrop)
}

// ---------------------------------------------------------------------------
// Scenario 1: network_jitter_db
//
// Invariant: Toxiproxy injects 500ms latency between portal and Postgres.
// DB-touching requests either succeed (with elevated latency) or surface a
// clear non-2xx status; no partial-state writes occur.
//
// Anti-tautology: a baseline sign-in is asserted to complete in under 5s
// before any toxic is injected, confirming the stack is healthy. If baseline
// is already slow, chaos results would be meaningless.
// ---------------------------------------------------------------------------

func testNetworkJitterDB(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	tp := toxiproxy.Start(ctx, t)
	mh := mailhog.Start(ctx, t)

	// Create a Toxiproxy proxy: toxiproxy-container port 22222 → postgres-container:5432.
	// The portal (a Docker container) connects to tp.ContainerIP:22222.
	// Toxiproxy forwards traffic to the Postgres container's bridge IP:5432.
	const (
		proxyName   = "pg"
		proxyPort   = 22222
		proxyListen = "0.0.0.0:22222"
	)
	pgContainerHost := netJitterExtractHost(pg.ContainerDSN)
	tp.CreateProxy(ctx, t, proxyName, proxyListen,
		fmt.Sprintf("%s:5432", pgContainerHost))

	// Wire portal's DB connection through Toxiproxy's bridge IP:22222.
	// No host-side port mapping is needed: both containers share the default
	// Docker bridge and can reach each other by container IP.
	dbName := netJitterExtractDBName(pg.ContainerDSN)
	containerDSN := fmt.Sprintf("postgres://test:test@%s:%d/%s?sslmode=disable",
		tp.ContainerIP, proxyPort, dbName)

	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     containerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	client := &http.Client{Timeout: 10 * time.Second}

	// ---- Baseline: assert sign-in completes quickly (no toxic yet) ----
	// If this exceeds 5s, the stack itself is too slow and the chaos
	// assertion would have no causal meaning.
	aliceEmail := randEmail(t, "alice-jitter")
	var alice authflow.TokenPair
	{
		start := time.Now()
		alice = authflow.SignInViaMagicLink(ctx, t, p, mh, aliceEmail)
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			t.Fatalf("network_jitter_db: baseline sign-in took %v (>5s); chaos test would be meaningless — is the stack too slow?", elapsed)
		}
		t.Logf("network_jitter_db: baseline sign-in elapsed: %v", elapsed)
	}

	// ---- Inject latency toxic ----
	// 500ms latency on the upstream direction (portal → Postgres).
	const toxicName = "latency_500ms"
	tp.AddLatency(ctx, t, proxyName, toxicName, 500)
	// Register cleanup so the toxic is removed even if the test fails early.
	// RemoveToxic will no-op (or warn) if already removed by the explicit call below.
	toxicRemoved := false
	t.Cleanup(func() {
		if !toxicRemoved {
			tp.RemoveToxic(context.Background(), t, proxyName, toxicName)
		}
	})

	// ---- Under-chaos assertion ----
	// GET /api/me is a DB lookup. Under 500ms latency the portal may succeed
	// (latency is tolerable) or surface 5xx (DB query timeout). What must NOT
	// happen is a 2xx response with corrupted or empty data.
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
		if err != nil {
			t.Fatalf("network_jitter_db: build /api/me request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+alice.AccessToken)

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			// A client-side timeout/connection error is acceptable: the portal did
			// not return a silent success with wrong data.
			t.Logf("network_jitter_db: GET /api/me under chaos: client error after %v: %v (portal did not silently succeed)", elapsed, err)
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			t.Logf("network_jitter_db: GET /api/me under chaos: status=%d elapsed=%v", resp.StatusCode, elapsed)

			if resp.StatusCode == http.StatusOK {
				// 200 is acceptable only if the body contains the correct data.
				var me struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				}
				if err := json.Unmarshal(body, &me); err != nil {
					t.Errorf("network_jitter_db: 200 response but body is not valid JSON: %v\nbody: %s", err, body)
				} else if me.Email != aliceEmail {
					t.Errorf("network_jitter_db: 200 response with wrong email: got %q, want %q", me.Email, aliceEmail)
				}
			} else if resp.StatusCode/100 == 2 {
				// Any other 2xx without correct data is a bug.
				t.Errorf("network_jitter_db: unexpected 2xx status %d under chaos", resp.StatusCode)
			} else {
				// Non-2xx (4xx, 5xx) is acceptable — portal surfaced the error.
				t.Logf("network_jitter_db: portal returned %d under chaos — acceptable (error surfaced)", resp.StatusCode)
			}
		}
	}

	// ---- Remove toxic and verify recovery ----
	tp.RemoveToxic(ctx, t, proxyName, toxicName)
	toxicRemoved = true

	deadline := time.Now().Add(20 * time.Second)
	var recovered bool
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/healthz", nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			recovered = true
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !recovered {
		t.Error("network_jitter_db: portal did not recover after removing toxic within 20s")
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: oauth_provider_timeout
//
// Invariant: the portal's GitHub OAuth HTTP client has a 15s timeout
// (githubOAuthHTTPTimeout in internal/portal/oauth/github.go). A 30s
// WireMock fixedDelayMilliseconds on /login/oauth/access_token forces the
// portal to time out at ~15s before WireMock would respond. The test
// asserts elapsed in a band around 15s and that the callback returns a
// non-2xx status.
// ---------------------------------------------------------------------------

func testOAuthProviderTimeout(t *testing.T) {
	ctx := context.Background()

	// WireMock: 30s fixedDelay on /login/oauth/access_token to simulate a
	// slow or hung OAuth provider. 30s > portal's 15s timeout, so the portal
	// timeout fires first.
	wm := wiremock.Start(ctx, t, wiremock.Mappings{
		"github-delay": oauthDelayMappingPath(),
	})

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
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

	// The HTTP client timeout must be set to portal's OAuth timeout + margin
	// so that we don't time out before the portal does, but well below
	// WireMock's 30s delay so that the portal timeout is the triggering event.
	const expectedPortalTimeout = 15 * time.Second
	client := &http.Client{Timeout: expectedPortalTimeout + 5*time.Second}

	// Start the OAuth flow to obtain a valid state nonce.
	authorizeURL := oauthStart(ctx, t, client, p.URL, "github")
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("oauth_provider_timeout: parse authorize_url: %v", err)
	}
	stateNonce := parsed.Query().Get("state")
	if stateNonce == "" {
		t.Fatalf("oauth_provider_timeout: no state param in authorize_url: %s", authorizeURL)
	}

	// Issue the callback — WireMock will delay the token exchange by 30s.
	// The portal's 15s OAuth HTTP client timeout fires first. The callback
	// must return a non-2xx status in a band around 15s: fast enough that
	// WireMock didn't respond first (lower bound ~14s), and not hanging past
	// the timeout window (upper bound ~18s).
	start := time.Now()
	status, body := oauthCallback(ctx, t, client, p.URL, "github", stateNonce, "chaos-code")
	elapsed := time.Since(start)

	const (
		lowerBound = 14 * time.Second // timeout fired (not a fast WireMock response)
		upperBound = 18 * time.Second // portal did not hang past timeout + 3s grace
	)
	if elapsed < lowerBound {
		t.Errorf("oauth_provider_timeout: callback returned in %v — too fast; portal likely did not exercise the 15s timeout (WireMock responded before timeout fired)", elapsed)
	}
	if elapsed > upperBound {
		t.Errorf("oauth_provider_timeout: callback took %v — portal hung beyond configured timeout + grace (%v)", elapsed, upperBound)
	}
	if status == http.StatusOK {
		t.Errorf("oauth_provider_timeout: portal returned 200 on a timed-out OAuth callback — expected error status\nbody: %s", body)
	}
	t.Logf("oauth_provider_timeout: callback returned status=%d elapsed=%v (expected non-2xx in [%v, %v])", status, elapsed, lowerBound, upperBound)
}

// ---------------------------------------------------------------------------
// Scenario 3: ws_reconnect_drop
//
// DEFERRED — depends on two backlog items:
//   - spa-websocket-reconnect-logic: SPA-side WS reconnect + UI missed-event
//     indicator.
//   - wsclient.ConnectFromSeq: Go test helper to subscribe from a given event
//     sequence number so reconnect can replay missed events.
//
// Until both are implemented this test is a documented placeholder.
// ---------------------------------------------------------------------------

func testWSReconnectDrop(t *testing.T) {
	t.Skip(
		"DEFERRED ws_reconnect_drop: requires spa-websocket-reconnect-logic " +
			"(SPA-side WS reconnect + missed-event indicator) and " +
			"wsclient.ConnectFromSeq Go helper. " +
			"Both tracked in .work/backlog/.",
	)
}

// ---------------------------------------------------------------------------
// Helpers shared across chaos_test files.
// These live here (not in runtime_and_clock_test.go) as noted in that file's
// comment. Go _test packages cannot import across binaries, so helpers that
// both chaos test files need must be co-located in the same package.
// ---------------------------------------------------------------------------

// randEmail returns a unique-per-run email address for parallel-safe inbox
// isolation. Uses crypto/rand; safe to call from concurrent sub-tests.
func randEmail(t *testing.T, prefix string) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("randEmail: rand.Read: %v", err)
	}
	return prefix + "-" + hex.EncodeToString(b) + "@example.com"
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}

// requirePortalImage skips t with an actionable message if the portal e2e
// image has not been built yet.
func requirePortalImage(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", "jamsesh/portal:e2e").Run(); err != nil {
		t.Skipf("portal e2e image %q not present — run `make test-portal-image` first", "jamsesh/portal:e2e")
	}
}

// ---------------------------------------------------------------------------
// File-local helpers (not duplicating anything in runtime_and_clock_test.go)
// ---------------------------------------------------------------------------

// oauthDelayMappingPath returns the absolute path to the WireMock mapping JSON
// that injects a 30s delay on /login/oauth/access_token.
func oauthDelayMappingPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "github_delay_30s.json")
}

// netJitterExtractDBName extracts the database name from a postgres DSN.
// e.g. "postgres://test:test@host:5432/testdb?sslmode=disable" → "testdb"
func netJitterExtractDBName(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "test"
	}
	return strings.TrimPrefix(u.Path, "/")
}

// netJitterExtractHost extracts the hostname (without port) from a postgres DSN.
// e.g. "postgres://test:test@172.17.0.3:5432/testdb?sslmode=disable" → "172.17.0.3"
func netJitterExtractHost(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "localhost"
	}
	return u.Hostname()
}

// oauthStart calls POST /api/auth/oauth/start and returns the authorize_url.
func oauthStart(ctx context.Context, t *testing.T, client *http.Client, baseURL, provider string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"provider": provider})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/auth/oauth/start", strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("oauthStart: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("oauthStart: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("oauthStart: status %d (want 200): %s", resp.StatusCode, respBody)
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("oauthStart: decode: %v\nbody: %s", err, respBody)
	}
	if out.AuthorizeURL == "" {
		t.Fatalf("oauthStart: empty authorize_url: %s", respBody)
	}
	return out.AuthorizeURL
}

// oauthCallback calls POST /api/auth/oauth/callback and returns (status, body).
func oauthCallback(ctx context.Context, t *testing.T, client *http.Client, baseURL, provider, state, code string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{
		"provider": provider,
		"state":    state,
		"code":     code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/auth/oauth/callback", strings.NewReader(string(b)))
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
