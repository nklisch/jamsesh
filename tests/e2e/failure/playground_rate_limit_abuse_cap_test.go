// Invariant: with JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR=180 (perMinute=3,
// burst=3), the first 3 anonymous POST /api/playground/sessions requests from
// the same source IP return 201 and the 4th returns 429 with a Retry-After
// header and a typed error envelope. A second client using a different
// X-Forwarded-For address is not affected by the first client's quota — the
// limiter is per-IP, not global.
//
// This test exercises the real rate-limit middleware wiring through the chi
// router, not the in-process httptest helpers used by the unit suite. A wiring
// bug (limiter constructed but not mounted on the route) would pass every unit
// test and ship a wide-open endpoint — this test catches that class of failure.
//
// Rate-limit arithmetic:
//
//	CreatePerIPHour=180 → perMinute = ceil(180/60) = 3 → burst = 3
//	Requests 1-3: within per-minute burst → 201 each.
//	Request 4:    burst exhausted → 429 + Retry-After.
package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestPlayground_RateLimit_FourthCreateBlocked verifies that the per-IP/hour
// playground session-create rate limiter fires against the real portal binary:
// requests 1-3 succeed (201) and the 4th is rejected (429) from the same
// client, while a second client with a distinct IP is unaffected.
func TestPlayground_RateLimit_FourthCreateBlocked(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	// Use CreatePerIPHour=180 so perMinute=3 and burst=3: the first 3 rapid-fire
	// requests are allowed and the 4th is blocked. With the default of 3/hour
	// (perMinute=1, burst=1) only the 1st request would succeed — we need the
	// 3-succeeds / 4th-fails shape to match the story's acceptance criterion.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		DBDSN:    pg.ContainerDSN,
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":              "true",
			"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR":   "180",
		},
	})

	t.Run("fourth_create_blocked", func(t *testing.T) {
		// Invariant: 3 sequential creates from the same client succeed, 4th is rejected.

		// Send requests from a stable spoofed IP via X-Forwarded-For. The portal
		// runs in behind_proxy mode (TLS_MODE=behind_proxy set by the fixture),
		// which enables chi's RealIP middleware — it re-writes r.RemoteAddr from
		// the leftmost X-Forwarded-For value when behind a trusted proxy. The
		// ratelimit.clientIP() function reads r.RemoteAddr after RealIP has
		// replaced it, so a consistent X-Forwarded-For simulates a stable client.
		clientIP := "10.99.1.1"

		for i := 1; i <= 3; i++ {
			resp := playgroundCreateWithIP(ctx, t, p, clientIP)
			resp.Body.Close()
			require.Equalf(t, http.StatusCreated, resp.StatusCode,
				"burst request %d from %s: want 201", i, clientIP)
		}

		// 4th request from the same IP must be rate-limited.
		resp4 := playgroundCreateWithIP(ctx, t, p, clientIP)
		body4, _ := io.ReadAll(resp4.Body)
		resp4.Body.Close()

		require.Equal(t, http.StatusTooManyRequests, resp4.StatusCode,
			"4th create from %s: want 429 (rate limited)\nbody: %s", clientIP, body4)

		// Retry-After must be a positive integer (seconds to wait before retry).
		retryAfter := resp4.Header.Get("Retry-After")
		require.NotEmpty(t, retryAfter,
			"4th create from %s: want Retry-After header on 429\nbody: %s", clientIP, body4)
		secs, err := strconv.Atoi(retryAfter)
		require.NoError(t, err,
			"Retry-After header %q must be an integer\nbody: %s", retryAfter, body4)
		require.Greater(t, secs, 0,
			"Retry-After header must be a positive integer, got %d\nbody: %s", secs, body4)

		// Error envelope must carry the typed error code.
		var env errorEnvelope
		require.NoError(t, json.Unmarshal(body4, &env),
			"4th create from %s: decode error envelope\nbody: %s", clientIP, body4)
		require.Equal(t, "rate_limited", env.Error,
			"4th create from %s: error envelope code\nbody: %s", clientIP, body4)

		t.Logf("rate_limit: 4th create correctly rejected: status=429 Retry-After=%s error=%q",
			retryAfter, env.Error)
	})

	t.Run("per_ip_isolation", func(t *testing.T) {
		// Invariant: exhausting one IP's quota does not affect a different IP.
		// We exhaust clientA's quota then verify clientB still gets 201.
		//
		// Both IPs route through the same portal instance. The per-IP limiter
		// maintains independent token-buckets per IP, so clientB's bucket is
		// fresh even after clientA has been blocked.
		//
		// Note: the burst size is 3 per IP. We consume 3 requests for clientA
		// (the 4th would block). Then we send 1 request from clientB. If the
		// limiter were global (not per-IP), clientB would see 429; if it is
		// correctly per-IP, clientB sees 201.
		clientA := "10.99.2.1"
		clientB := "10.99.3.1"

		// Exhaust clientA's burst quota.
		for i := 1; i <= 3; i++ {
			resp := playgroundCreateWithIP(ctx, t, p, clientA)
			resp.Body.Close()
		}

		// Confirm clientA is now blocked.
		respA4 := playgroundCreateWithIP(ctx, t, p, clientA)
		respA4.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, respA4.StatusCode,
			"clientA 4th create: want 429 (limiter must be active for clientA)")

		// clientB — a fresh IP — must still be allowed.
		respB := playgroundCreateWithIP(ctx, t, p, clientB)
		bodyB, _ := io.ReadAll(respB.Body)
		respB.Body.Close()
		require.Equal(t, http.StatusCreated, respB.StatusCode,
			"clientB 1st create: want 201 (clientB has a fresh per-IP bucket)\nbody: %s", bodyB)

		t.Logf("per_ip_isolation: clientB correctly got 201 while clientA is at 429")
	})
}

// playgroundCreateWithIP sends POST /api/playground/sessions with the given
// X-Forwarded-For IP. The caller is responsible for closing resp.Body.
func playgroundCreateWithIP(ctx context.Context, t *testing.T, p *portal.Portal, clientIP string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.URL+"/api/playground/sessions", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("playgroundCreateWithIP: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The portal's chi RealIP middleware rewrites r.RemoteAddr from the leftmost
	// X-Forwarded-For entry. Setting this header simulates requests from distinct
	// client IPs routed through the same reverse proxy.
	req.Header.Set("X-Forwarded-For", clientIP)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playgroundCreateWithIP: POST: %v", err)
	}
	return resp
}
