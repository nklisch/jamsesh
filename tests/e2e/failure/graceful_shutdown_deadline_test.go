// Invariant: the portal's graceful-shutdown deadline (JAMSESH_SHUTDOWN_GRACE_S)
// is honoured in both directions:
//
//  1. request_finishes_within_deadline — an in-flight OAuth callback that
//     completes before the deadline finishes normally even after SIGTERM.
//
//  2. request_exceeds_deadline — an in-flight OAuth callback that would
//     outlast the deadline is cut off at the deadline boundary; total elapsed
//     is bounded close to the configured grace period.
//
// Both subtests drive the portal via its real OAuth callback path
// (POST /api/auth/oauth/callback) with a WireMock-injected delay on
// /login/oauth/access_token — the same mechanism used by
// tests/e2e/chaos/network_and_provider_test.go > testOAuthProviderTimeout.
//
// Shutdown race note: the known data-race in cmd/portal/main.go
// (graceful-shutdown-shutdownstart-race) is benign in practice and does not
// affect these tests because the e2e suite does not run with -race. If that
// race surfaces as a flake or panic, add t.Skip with the item id and park a
// blocker rather than working around it here.
package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wiremock"
)

// TestGracefulShutdownDeadline exercises both sides of the shutdown-deadline
// contract: in-flight requests that fit inside the grace window complete; those
// that exceed it are cut off near the deadline boundary.
func TestGracefulShutdownDeadline(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	t.Run("request_finishes_within_deadline", testShutdownRequestFinishesWithinDeadline)
	t.Run("request_exceeds_deadline", testShutdownRequestExceedsDeadline)
}

// testShutdownRequestFinishesWithinDeadline asserts that a 2s OAuth callback
// completes successfully when the shutdown deadline is 10s.
//
// Timeline:
//
//	t=0s   portal starts (JAMSESH_SHUTDOWN_GRACE_S=10)
//	t=0s   WireMock delays /login/oauth/access_token by 2s
//	t=0s   in-flight POST /api/auth/oauth/callback starts
//	t=200ms SIGTERM sent — grace window opens
//	t=2s   WireMock responds → portal completes the request normally
//	t=2s   portal drains and exits (well within the 10s deadline)
//
// Invariant: the HTTP request completes without a connection-level error.
// A test that "passes" because SIGTERM immediately kills the connection
// violates this invariant.
func testShutdownRequestFinishesWithinDeadline(t *testing.T) {
	ctx := context.Background()

	wm := wiremock.Start(ctx, t, wiremock.Mappings{
		"oauth-delay-2s": shutdownMappingPath(t, "oauth_delay_2s.json"),
	})

	pg := postgres.Start(ctx, t, postgres.Options{})

	p := portal.Start(ctx, t, portal.Options{
		DBDriver:                "postgres",
		DBDSN:                   pg.ContainerDSN,
		EmailFrom:               "noreply@example.com",
		OAuthBaseURL:            wm.ContainerURL,
		OAuthGitHubClientID:     "test-client",
		OAuthGitHubClientSecret: "test-secret",
		ExtraEnv: map[string]string{
			// Deadline of 10s >> WireMock delay of 2s.
			// The in-flight request must complete before the deadline fires.
			"JAMSESH_SHUTDOWN_GRACE_S": "10",
		},
	})

	// Obtain a valid state nonce via /api/auth/oauth/start so the callback
	// request is not immediately rejected with a nonce-mismatch error.
	// The HTTP client needs a generous timeout here — the portal is healthy
	// and the start endpoint doesn't touch WireMock.
	startClient := &http.Client{Timeout: 10 * time.Second}
	stateNonce := shutdownOAuthStart(ctx, t, startClient, p.URL, "github")

	// Launch the callback in a goroutine. WireMock will hold the response for
	// 2s. Meanwhile we send SIGTERM, opening the 10s grace window.
	type result struct {
		status int
		err    error
	}
	done := make(chan result, 1)

	// Use a client with no timeout — the test's outer select provides the
	// deadline. The portal will close the connection either on completion or
	// when the grace deadline fires, so this call will always unblock.
	callbackClient := &http.Client{}
	go func() {
		s, _, err := shutdownOAuthCallbackRaw(ctx, callbackClient, p.URL, "github", stateNonce, "shutdown-code")
		done <- result{s, err}
	}()

	// Wait briefly so the callback is in-flight before we signal.
	time.Sleep(200 * time.Millisecond)

	require.NoError(t, p.SendSignal(ctx, syscall.SIGTERM),
		"SIGTERM must reach PID 1 in the container")

	// The request must complete (either 2xx or error-class 4xx/5xx) without
	// a connection-level error. A transport-layer error here means the portal
	// killed the connection immediately on SIGTERM — failing the invariant.
	select {
	case r := <-done:
		require.NoError(t, r.err,
			"in-flight request must complete within the 10s grace window — "+
				"a connection error means the portal killed it immediately on SIGTERM")
		t.Logf("request_finishes_within_deadline: in-flight request returned status %d (graceful)", r.status)
	case <-time.After(15 * time.Second):
		t.Fatal("request_finishes_within_deadline: in-flight request did not complete within 15s — portal did not drain")
	}
}

// testShutdownRequestExceedsDeadline asserts that a 10s OAuth callback is cut
// off near the 2s deadline and that elapsed time is bounded.
//
// Timeline:
//
//	t=0s   portal starts (JAMSESH_SHUTDOWN_GRACE_S=2)
//	t=0s   WireMock delays /login/oauth/access_token by 10s
//	t=0s   in-flight POST /api/auth/oauth/callback starts
//	t=200ms SIGTERM sent — grace window opens (2s)
//	t=2.2s  deadline fires → portal forcibly closes connections and exits
//
// Invariant A: total elapsed from SIGTERM to request completion is < 4s
//
//	(deadline + 1.8s margin for signal delivery overhead and
//	TCP RST propagation).
//
// Invariant B: either a connection-level error or a non-2xx status is
//
//	returned — a 2xx would mean the portal served a fake response before
//	WireMock's 10s delay completed, which cannot happen legitimately.
func testShutdownRequestExceedsDeadline(t *testing.T) {
	ctx := context.Background()

	wm := wiremock.Start(ctx, t, wiremock.Mappings{
		"oauth-delay-10s": shutdownMappingPath(t, "oauth_delay_10s.json"),
	})

	pg := postgres.Start(ctx, t, postgres.Options{})

	p := portal.Start(ctx, t, portal.Options{
		DBDriver:                "postgres",
		DBDSN:                   pg.ContainerDSN,
		EmailFrom:               "noreply@example.com",
		OAuthBaseURL:            wm.ContainerURL,
		OAuthGitHubClientID:     "test-client",
		OAuthGitHubClientSecret: "test-secret",
		ExtraEnv: map[string]string{
			// Deadline of 2s << WireMock delay of 10s.
			// The deadline must fire and cut off the in-flight request.
			"JAMSESH_SHUTDOWN_GRACE_S": "2",
		},
	})

	// Obtain a valid state nonce before we SIGTERM the portal.
	startClient := &http.Client{Timeout: 10 * time.Second}
	stateNonce := shutdownOAuthStart(ctx, t, startClient, p.URL, "github")

	type result struct {
		status int
		err    error
	}
	done := make(chan result, 1)
	callbackClient := &http.Client{}
	go func() {
		s, _, err := shutdownOAuthCallbackRaw(ctx, callbackClient, p.URL, "github", stateNonce, "shutdown-code")
		done <- result{s, err}
	}()

	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	require.NoError(t, p.SendSignal(ctx, syscall.SIGTERM),
		"SIGTERM must reach PID 1 in the container")

	// Invariant A: the request terminates within deadline + margin.
	const (
		gracePeriod  = 2 * time.Second
		signalMargin = 200 * time.Millisecond // time.Sleep above
		cutoffMargin = 2 * time.Second        // TCP RST delivery + test overhead
		cutoff       = signalMargin + gracePeriod + cutoffMargin
	)

	select {
	case r := <-done:
		elapsed := time.Since(start)

		// Invariant A: terminated near the deadline.
		require.Less(t, elapsed, cutoff,
			"request_exceeds_deadline: elapsed %v must be < %v (deadline %v + margin %v); "+
				"portal may have hung past the deadline",
			elapsed, cutoff, gracePeriod, cutoffMargin)

		// Invariant B: connection error OR non-2xx — not a successful completion.
		if r.err != nil {
			t.Logf("request_exceeds_deadline: request terminated with connection error after %v: %v (deadline fired — correct)", elapsed, r.err)
		} else {
			require.NotEqual(t, http.StatusOK, r.status,
				"request_exceeds_deadline: portal returned 200 on a request that should have been cut off by the %v deadline; "+
					"WireMock delay was 10s so a legitimate 200 is impossible here",
				gracePeriod)
			t.Logf("request_exceeds_deadline: request returned status %d after %v (non-2xx is expected; deadline fired)", r.status, elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("request_exceeds_deadline: request did not terminate within 15s — deadline did not fire; portal may be hanging")
	}
}

// ---------------------------------------------------------------------------
// File-local helpers
// ---------------------------------------------------------------------------

// shutdownMappingPath returns the absolute host path for a WireMock mapping
// JSON file in this package's testdata directory.
func shutdownMappingPath(t *testing.T, filename string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", filename)
}

// shutdownOAuthStart calls POST /api/auth/oauth/start and returns the state
// nonce. It is a local variant of oauthStartOAuth (defined in
// config_and_deps_test.go) to keep this file self-contained for the signal
// path.
func shutdownOAuthStart(ctx context.Context, t *testing.T, client *http.Client, baseURL, provider string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"provider": provider})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/auth/oauth/start", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("shutdownOAuthStart: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("shutdownOAuthStart: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("shutdownOAuthStart: status %d (want 200): %s", resp.StatusCode, body)
	}
	var out struct {
		AuthorizeURL string `json:"authorize_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("shutdownOAuthStart: decode: %v\nbody: %s", err, body)
	}
	parsed, err := url.Parse(out.AuthorizeURL)
	if err != nil {
		t.Fatalf("shutdownOAuthStart: parse authorize_url %q: %v", out.AuthorizeURL, err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("shutdownOAuthStart: no state param in authorize_url: %s", out.AuthorizeURL)
	}
	return state
}

// shutdownOAuthCallbackRaw issues POST /api/auth/oauth/callback without
// calling t.Fatalf on transport errors. The caller decides how to handle
// connection errors — in the deadline-exceeded case a connection error is the
// expected outcome.
func shutdownOAuthCallbackRaw(ctx context.Context, client *http.Client, baseURL, provider, state, code string) (int, []byte, error) {
	b, _ := json.Marshal(map[string]string{
		"provider": provider,
		"state":    state,
		"code":     code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/auth/oauth/callback", bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}
