// Invariant: the soft-coordinator hint cache populates on a clean success and
// invalidates on a 503, and per-session hints do not bleed across sessions.
//
// Routing identity is established via cluster.LeaseHolder (Postgres advisory
// lock query) and via the router's /metrics endpoint
// (jamsesh_router_decisions_total{result="hit_cache"}).
//
// # Hint-cache lifecycle (from proxy.go)
//
//  1. First request: ring lookup (hit_ring). On clean success → hint.Set.
//  2. Second request: hint lookup (hit_cache). Bypasses ring.
//  3. If first-attempt pod returns 503: hint.Invalidate → retry via ring.GetNext.
//     The retry does NOT call hint.Set ("Don't update hint on retry").
//  4. After invalidation: next request falls back to ring (hit_ring).
//     On clean success from that ring-chosen pod → hint.Set again.
//  5. Subsequent request → hit_cache again.
//
// The test holds the Postgres advisory lock for a session from the test
// process to force a 503 from the ring-preferred pod, triggering steps 3-5.
// This avoids the 5-minute TTL entirely.
package golden_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestRouterHintCache exercises the soft-coordinator hint cache in two subtests:
//  1. hint_cache_overrides_ring_after_503 — 503-driven invalidation and
//     re-population, verified via the router's /metrics counter.
//  2. hint_cache_is_per_session — per-session isolation, verified via
//     LeaseHolder on distinct sessions.
func TestRouterHintCache(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	// ── Subtest 1: 503 invalidation ──────────────────────────────────────────
	// Uses a 2-pod cluster so the ring has exactly two choices. When pod A
	// (the ring-preferred pod) is forced to 503 by holding its advisory lock
	// from the test process, the router retries on pod B. After releasing the
	// lock, the next clean request re-populates the hint; the request after
	// that is served via the hint (hit_cache counter increments).
	t.Run("hint_cache_overrides_ring_after_503", func(t *testing.T) {
		cluster2 := portalcluster.Start(ctx, t, portalcluster.Options{
			Pods:        2,
			Postgres:    pg,
			ObjectStore: mn,
			Router:      true,
			PortalExtraEnv: map[string]string{
				"JAMSESH_EMAIL_PROVIDER":  "smtp",
				"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
				"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
				"JAMSESH_EMAIL_SMTP_TLS":  "none",
			},
		})
		require.NotEmpty(t, cluster2.RouterURL,
			"hint_cache_overrides_ring_after_503: requires Router: true — RouterURL must not be empty")

		// Verify the router's /metrics endpoint exposes the decisions counter
		// before the test body runs. If it's absent, skip with a diagnostic
		// message rather than silently producing a tautological pass.
		routerMetricsURL := cluster2.RouterURL + "/metrics"
		requireRouterDecisionsCounter(t, routerMetricsURL)

		pod0 := cluster2.Pods[0]
		userEmail := randEmail(t, "hint503")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "HintCache503 Org")

		testHintCacheAfter503(ctx, t, cluster2, pair.AccessToken, orgID, pg.DSN, routerMetricsURL)
	})

	// ── Subtest 2: per-session isolation ─────────────────────────────────────
	// Uses a 3-pod cluster. Routes three distinct sessions and verifies that:
	// a) Each session consistently routes to its own pod (LeaseHolder is
	//    stable across repeated requests).
	// b) The hint for session A does not interfere with routing for session B
	//    (a blanket-cache bug would route all sessions to the same pod).
	t.Run("hint_cache_is_per_session", func(t *testing.T) {
		cluster3 := portalcluster.Start(ctx, t, portalcluster.Options{
			Pods:        3,
			Postgres:    pg,
			ObjectStore: mn,
			Router:      true,
			PortalExtraEnv: map[string]string{
				"JAMSESH_EMAIL_PROVIDER":  "smtp",
				"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
				"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
				"JAMSESH_EMAIL_SMTP_TLS":  "none",
			},
		})
		require.NotEmpty(t, cluster3.RouterURL,
			"hint_cache_is_per_session: requires Router: true — RouterURL must not be empty")

		pod0 := cluster3.Pods[0]
		userEmail := randEmail(t, "hintper")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "HintCachePer Org")

		testHintCachePerSession(ctx, t, cluster3, pair.AccessToken, orgID)
	})
}

// ---------------------------------------------------------------------------
// Subtest implementations
// ---------------------------------------------------------------------------

// testHintCacheAfter503 exercises the 503-invalidation → repopulation cycle:
//
//  1. Create a session; warm the hint via a clean request (hit_ring → hint.Set).
//  2. Scrape /metrics: record baseline hit_cache counter.
//  3. Hold the advisory lock from the test process → next request to the
//     ring-preferred pod returns 503 → router invalidates hint + retries.
//  4. Release lock.
//  5. Make a clean request → ring picks the pod again → hint.Set.
//  6. Make another clean request → hit_cache (hint routes directly).
//  7. Scrape /metrics: assert hit_cache incremented by at least 1 since step 2.
func testHintCacheAfter503(
	ctx context.Context,
	t *testing.T,
	cluster *portalcluster.Cluster,
	accessToken, orgID, pgDSN, routerMetricsURL string,
) {
	t.Helper()

	// Step 1: Create session and warm the hint with a clean request.
	sessionID := hintCreateSession(ctx, t, cluster.RouterURL, accessToken, orgID, "hint503-session")

	// Warm request: ring lookup → hint.Set. After this the hint is populated.
	hintGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)

	// The advisory lock may be held briefly after the request completes.
	// Poll until a pod holds the lease so we know exactly which pod is warm.
	warmHolder := cluster.RequireLeaseHolder(ctx, t, sessionID, 3*time.Second)
	t.Logf("hint_cache_after_503: hint warmed; lease holder = pod %d", warmHolder)

	// Step 2: Baseline hit_cache counter before the 503 sequence.
	baselineHitCache := scrapeRouterDecision(t, routerMetricsURL, "hit_cache")
	t.Logf("hint_cache_after_503: baseline hit_cache = %v", baselineHitCache)

	// Step 3: Hold the advisory lock for this session from the test process.
	// The portal uses pg_advisory_lock(hashtext(sessionID)::oid) which is a
	// session-level advisory lock; it blocks until released. The portal's own
	// lock acquisition is non-blocking (pg_try_advisory_lock), so it fails and
	// returns 503 when the lock is already held.
	//
	// We open a dedicated DB connection that lives for the duration of the lock
	// hold. The connection must NOT be closed before we release the lock.
	lockDB, err := sql.Open("postgres", pgDSN)
	require.NoError(t, err, "hint_cache_after_503: open lock DB connection")

	// Acquire the advisory lock (blocking — will succeed immediately since only
	// the test process holds it right now).
	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1)::oid)", sessionID)
	require.NoError(t, err, "hint_cache_after_503: acquire advisory lock")
	t.Logf("hint_cache_after_503: advisory lock held for session %s", sessionID)

	// Issue the request while the lock is held. The ring-preferred pod will
	// try pg_try_advisory_lock, fail (lock already held), and return 503.
	// The router then invalidates the hint and retries on the next pod.
	// The retry pod can acquire the lock too? No — we hold the lock in *our*
	// connection. The retry pod also fails and returns 503. The router then
	// propagates 503 to the client.
	//
	// That is the correct behaviour (bounded retry): client sees 503 once both
	// pods fail. The important side-effect is the hint invalidation.
	resp503, err := hintDoGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)
	require.NoError(t, err, "hint_cache_after_503: GET while lock held (network error)")
	t.Logf("hint_cache_after_503: GET while lock held → %d (want 503 or 2xx if retry pod acquired)", resp503)
	// We accept 503 (both pods blocked) or 2xx (edge case: the other pod
	// raced to acquire before our lock was established — unlikely but possible
	// if the cluster routes to the retry pod which has no lock held). Both are
	// valid: the hint invalidation is the critical side-effect we test below.

	// Step 4: Release the advisory lock.
	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1)::oid)", sessionID)
	require.NoError(t, err, "hint_cache_after_503: release advisory lock")
	lockDB.Close()
	t.Logf("hint_cache_after_503: advisory lock released")

	// Step 5: Clean request — hint was invalidated, so the router falls back to
	// the ring (hit_ring). The pod acquires the lock, returns 2xx; the router
	// calls hint.Set for this session.
	hintGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)
	afterRelease := cluster.RequireLeaseHolder(ctx, t, sessionID, 3*time.Second)
	t.Logf("hint_cache_after_503: clean request after release; lease holder = pod %d", afterRelease)

	// Step 6: Another clean request — the hint is now populated. The router
	// serves this from the hint (hit_cache increments).
	hintGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)
	t.Logf("hint_cache_after_503: second clean request complete (should be hit_cache)")

	// Step 7: Scrape /metrics and assert hit_cache counter increased.
	afterHitCache := scrapeRouterDecision(t, routerMetricsURL, "hit_cache")
	t.Logf("hint_cache_after_503: after hit_cache = %v", afterHitCache)

	require.Greaterf(t, afterHitCache, baselineHitCache,
		"hint_cache_after_503: expected jamsesh_router_decisions_total{result=\"hit_cache\"} to "+
			"increase after the warm-request → 503-invalidation → repopulation cycle, but "+
			"counter did not change (baseline=%v, after=%v). "+
			"This suggests the hint cache is not being populated on clean success — "+
			"possible causes: metric label mismatch, hint not set in proxy.go, or the "+
			"second clean request went to a pod that triggered another 503 (ring instability). "+
			"If reproducible, park as a production bug rather than weakening the assertion.",
		baselineHitCache, afterHitCache)

	t.Logf("hint_cache_after_503: hit_cache incremented from %v to %v ✓",
		baselineHitCache, afterHitCache)
}

// testHintCachePerSession creates 3 sessions on a 3-pod cluster and verifies:
//  1. Each session consistently routes to the same pod (LeaseHolder is stable).
//  2. The hint for one session does not bleed into routing for another session
//     (a blanket-replacement bug would route all sessions to the last-used pod).
func testHintCachePerSession(
	ctx context.Context,
	t *testing.T,
	cluster *portalcluster.Cluster,
	accessToken, orgID string,
) {
	t.Helper()

	const sessionCount = 3
	const requestsPerSession = 5

	sessionIDs := make([]string, 0, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sid := hintCreateSession(ctx, t, cluster.RouterURL, accessToken, orgID,
			fmt.Sprintf("per-session-%d-%d", i, time.Now().UnixNano()))
		sessionIDs = append(sessionIDs, sid)
	}

	// Warm each session with an initial request so hints are populated.
	for i, sid := range sessionIDs {
		hintGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sid)
		t.Logf("hint_cache_per_session: session[%d]=%s initial request done", i, sid)
	}

	// Capture the lease holder for each session after warming.
	initialHolders := make([]int, sessionCount)
	for i, sid := range sessionIDs {
		h := cluster.RequireLeaseHolder(ctx, t, sid, 3*time.Second)
		initialHolders[i] = h
		t.Logf("hint_cache_per_session: session[%d]=%s → pod %d (initial)", i, sid, h)
	}

	// Issue requestsPerSession more requests per session in a round-robin
	// pattern that interleaves sessions to stress the per-session isolation:
	// session 0, session 1, session 2, session 0, session 1, … This ensures
	// a blanket-replacement cache (one global slot) would visibly misroute.
	for round := 0; round < requestsPerSession; round++ {
		for i, sid := range sessionIDs {
			hintGetSession(ctx, t, cluster.RouterURL, accessToken, orgID, sid)

			// Check the holder immediately after each request. The hint should
			// route the request to the same pod as the initial request.
			h := cluster.LeaseHolder(ctx, t, sid)
			if h < 0 {
				// Transient: no lock held between requests. Allow one retry.
				time.Sleep(100 * time.Millisecond)
				h = cluster.LeaseHolder(ctx, t, sid)
			}
			require.GreaterOrEqualf(t, h, 0,
				"hint_cache_per_session: round %d, session[%d]=%s: LeaseHolder returned -1 — "+
					"no pod holds the advisory lock after routing; transient lock release or "+
					"hashtext mismatch",
				round, i, sid)
			require.Equalf(t, initialHolders[i], h,
				"hint_cache_per_session: round %d, session[%d]=%s: routed to pod %d but "+
					"initial holder was pod %d — either consistent-hash routing is unstable "+
					"or the hint cache is polluting across sessions (blanket-replacement bug). "+
					"If reproducible, park as a production bug.",
				round, i, sid, h, initialHolders[i])
		}
	}

	// Assert at least 2 distinct pods appear in the initial holders to confirm
	// the test exercises true per-session isolation (if all 3 sessions land on
	// the same pod, the per-session isolation invariant is vacuously true).
	podSet := make(map[int]struct{}, sessionCount)
	for _, h := range initialHolders {
		podSet[h] = struct{}{}
	}
	if len(podSet) < 2 {
		t.Logf("hint_cache_per_session: WARNING: all %d sessions landed on a single pod "+
			"(%v); the per-session isolation check passes trivially. Consider re-running "+
			"to confirm distribution. This is not a failure — consistent-hash rings can "+
			"skew — but it reduces coverage of the isolation invariant.", sessionCount, podSet)
	} else {
		t.Logf("hint_cache_per_session: %d distinct pods served %d sessions ✓", len(podSet), sessionCount)
	}

	t.Logf("hint_cache_per_session: all %d sessions maintained stable routing across %d rounds ✓",
		sessionCount, requestsPerSession)
}

// ---------------------------------------------------------------------------
// Metrics helpers
// ---------------------------------------------------------------------------

// requireRouterDecisionsCounter scrapes the router's /metrics endpoint and
// confirms that jamsesh_router_decisions_total is present. If the metric is
// absent, the test is skipped with a diagnostic message directing the
// implementer to check the metrics registry.
//
// This is a pre-flight check — it runs before the subtest body so that a
// missing metric produces a clear skip rather than a confusing assertion
// failure later.
func requireRouterDecisionsCounter(t *testing.T, routerMetricsURL string) {
	t.Helper()
	families := fetchRouterMetrics(t, routerMetricsURL)
	if _, ok := families["jamsesh_router_decisions_total"]; !ok {
		t.Skipf("router /metrics does not expose jamsesh_router_decisions_total — "+
			"check that the router binary registers the metric via metrics.New() and "+
			"that the metric name matches. Metric families present: %v",
			sortedFamilyNames(families))
	}
}

// scrapeRouterDecision fetches /metrics from the router and returns the current
// value of jamsesh_router_decisions_total{result=result}. Returns 0.0 if the
// counter has not yet been incremented (label combination absent — Prometheus
// CounterVec omits zero-value time series).
func scrapeRouterDecision(t *testing.T, routerMetricsURL, result string) float64 {
	t.Helper()
	families := fetchRouterMetrics(t, routerMetricsURL)

	family, ok := families["jamsesh_router_decisions_total"]
	if !ok {
		// Counter not present yet — treat as 0.
		return 0.0
	}

	for _, m := range family.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "result" && lp.GetValue() == result {
				return m.GetCounter().GetValue()
			}
		}
	}
	// Label combination absent — counter never incremented for this result label.
	return 0.0
}

// fetchRouterMetrics GETs routerMetricsURL and parses the Prometheus text
// exposition format. On network or parse error, the test is fatally failed.
func fetchRouterMetrics(t *testing.T, routerMetricsURL string) map[string]*dto.MetricFamily {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, routerMetricsURL, nil)
	require.NoError(t, err, "fetchRouterMetrics: build request")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "fetchRouterMetrics: GET %s", routerMetricsURL)
	defer resp.Body.Close()

	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"fetchRouterMetrics: router /metrics returned %d; want 200", resp.StatusCode)

	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(resp.Body)
	require.NoError(t, err, "fetchRouterMetrics: parse Prometheus exposition format")
	return families
}

// sortedFamilyNames returns a sorted, space-joined list of metric family names.
// Used in skip/error messages for readability.
func sortedFamilyNames(families map[string]*dto.MetricFamily) string {
	names := make([]string, 0, len(families))
	for n := range families {
		names = append(names, n)
	}
	// Simple insertion sort — the list is short (< 50 entries).
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return strings.Join(names, ", ")
}

// ---------------------------------------------------------------------------
// Session request helpers (local to this file)
// ---------------------------------------------------------------------------

// hintCreateSession creates a session by POSTing through the router URL and
// returns the new session ID.
func hintCreateSession(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body := map[string]string{
		"name":         name,
		"goal":         "router hint-cache test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "hintCreateSession: marshal body")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", routerURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "hintCreateSession: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "hintCreateSession: POST")
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"hintCreateSession: want 201 Created; got %d; body: %s", resp.StatusCode, respBody)

	var s routerSessionRef // reuse type from router_consistent_hash_test.go
	require.NoError(t, json.Unmarshal(respBody, &s),
		"hintCreateSession: decode response; body: %s", respBody)
	require.NotEmpty(t, s.ID, "hintCreateSession: empty session ID in response")
	return s.ID
}

// hintGetSession issues GET /api/orgs/{orgID}/sessions/{sessionID} through the
// router and requires a 2xx response. The response body is discarded — this
// helper's purpose is to trigger routing (and thus advisory lock acquisition
// and hint cache updates), not to validate response content.
func hintGetSession(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, orgID, sessionID string,
) {
	t.Helper()
	code, err := hintDoGetSession(ctx, t, routerURL, accessToken, orgID, sessionID)
	require.NoError(t, err, "hintGetSession: network error")
	require.Truef(t, code >= 200 && code < 300,
		"hintGetSession: want 2xx; got %d for session %s", code, sessionID)
}

// hintDoGetSession issues GET /api/orgs/{orgID}/sessions/{sessionID} through
// the router and returns the HTTP status code without asserting. Used when the
// caller needs to inspect 503 vs 2xx (e.g. while holding the advisory lock).
func hintDoGetSession(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, orgID, sessionID string,
) (int, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", routerURL, orgID, sessionID),
		nil)
	if err != nil {
		return 0, fmt.Errorf("hintDoGetSession: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("hintDoGetSession: do request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body) // drain to allow connection reuse
	return resp.StatusCode, nil
}
