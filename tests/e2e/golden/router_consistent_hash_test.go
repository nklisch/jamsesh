// Invariant: same session_id consistently routes to the same backend pod
// absent re-ring events; different session IDs distribute across pods.
//
// Routing identity is established via cluster.LeaseHolder, which queries
// Postgres pg_locks to find which pod holds the advisory lock for a given
// session. Every assertion in this file is anchored to LeaseHolder — not
// to response bodies or per-pod headers.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// routerSessionRef is the minimal create-session response used in this file.
type routerSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestRouterConsistentHash is the golden test for the consistent-hash routing
// invariant. It exercises the router, not individual pods, and verifies that
// routing identity (which pod holds the advisory lock for a session) matches
// the expected consistent-hash properties.
func TestRouterConsistentHash(t *testing.T) {
	// SKIP: idea-router-e2e-lease-premise. Both subtests anchor routing identity
	// to cluster.LeaseHolder, which queries pg_locks for the per-session advisory
	// lock. But the portal acquires that lock only via the git/objectstore
	// LifecycleManager (wired into the git smart-HTTP handler in
	// cmd/portal/main.go); the REST GetSession path these tests drive reads
	// Postgres directly and never takes the advisory lock. LeaseHolder therefore
	// always returns -1 ("-1 not >= 0"), independent of router correctness. The
	// consistent-hash ring + hint cache product code is correct and unit-tested
	// (internal/router/ring, internal/router/proxy). Re-anchoring this golden test
	// to a lease-bearing signal (git ops through the router, a per-pod identity
	// header, or the /metrics decisions counter) is tracked in the backlog item;
	// it overlaps the separately-red cross-pod git-serving path. Do NOT weaken the
	// LeaseHolder assertions to make this green — that would lie about routing.
	t.Skip("router consistent-hash golden test is blocked on idea-router-e2e-lease-premise: " +
		"REST GETs do not acquire the per-session advisory lease, so cluster.LeaseHolder " +
		"returns -1 regardless of router correctness; the test must be re-anchored to a " +
		"lease-bearing routing signal (see .work/backlog/idea-router-e2e-lease-premise.md)")

	// ── Infrastructure ───────────────────────────────────────────────────────
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
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
	require.NotEmpty(t, cluster.RouterURL,
		"consistent-hash test requires Router: true — RouterURL must not be empty")

	// Auth uses pod 0 directly (all pods share Postgres; token is valid
	// cluster-wide). The router is used only for session-scoped requests so
	// the consistent-hash routing layer exercises its session-ID extraction.
	pod0 := cluster.Pods[0]
	userEmail := randEmail(t, "hash")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)

	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Hash Routing Org")

	t.Run("same_session_pins_to_same_pod", func(t *testing.T) {
		testSameSessionPinsToPod(ctx, t, cluster, pair.AccessToken, orgID)
	})

	t.Run("different_sessions_distribute", func(t *testing.T) {
		testDifferentSessionsDistribute(ctx, t, cluster, pair.AccessToken, orgID)
	})
}

// testSameSessionPinsToPod creates one session, issues N≥20 GET requests for
// that session through the router, and asserts that LeaseHolder returns the
// same pod index for every request.
//
// Each GET request is to GET /api/orgs/{orgID}/sessions/{sessionID}, which
// the router extracts via its REST session-ID parser. The portal's lease
// layer acquires the advisory lock on the first request and holds it for the
// session's lifetime, so every subsequent request from the router lands on
// the same pod — that is the invariant under test.
func testSameSessionPinsToPod(
	ctx context.Context,
	t *testing.T,
	cluster *portalcluster.Cluster,
	accessToken, orgID string,
) {
	t.Helper()

	sessionID := createSessionViaRouterURL(ctx, t, cluster.RouterURL, accessToken, orgID,
		"pin-test-session")

	const requestCount = 20

	// Issue the first request and capture the initial lease holder.
	getSessionViaRouter(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)

	firstHolder := cluster.LeaseHolder(ctx, t, sessionID)
	require.GreaterOrEqualf(t, firstHolder, 0,
		"same_session_pins_to_same_pod: LeaseHolder returned -1 after first request for session %s — "+
			"no pod holds the advisory lock; this indicates either the portal did not acquire the lock "+
			"(possible hashtext portability issue) or the lock was released before the query",
		sessionID)
	t.Logf("same_session_pins_to_same_pod: first request routed to pod %d", firstHolder)

	// Issue remaining requests and assert the holder is stable.
	for i := 1; i < requestCount; i++ {
		getSessionViaRouter(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)

		holder := cluster.LeaseHolder(ctx, t, sessionID)
		require.GreaterOrEqualf(t, holder, 0,
			"same_session_pins_to_same_pod: request %d: LeaseHolder returned -1 for session %s — "+
				"no pod holds the lock between requests; transient lock release or hashtext mismatch",
			i+1, sessionID)
		require.Equalf(t, firstHolder, holder,
			"same_session_pins_to_same_pod: request %d: session %s routed to pod %d but first request "+
				"went to pod %d — consistent-hash invariant violated",
			i+1, sessionID, holder, firstHolder)
	}

	t.Logf("same_session_pins_to_same_pod: all %d requests routed to pod %d ✓", requestCount, firstHolder)
}

// testDifferentSessionsDistribute creates K≥10 distinct sessions, routes one
// request each through the router, and asserts that at least 2 distinct pod
// indices appear across the K sessions.
//
// With 3 pods and 10 sessions the probability of all landing on one pod via
// consistent hashing is negligible but the assertion is conservative ("at
// least 2 pods") to avoid test brittleness from minor ring-balance skew.
func testDifferentSessionsDistribute(
	ctx context.Context,
	t *testing.T,
	cluster *portalcluster.Cluster,
	accessToken, orgID string,
) {
	t.Helper()

	const sessionCount = 10

	// Create all sessions first (POST through the router), then issue one GET
	// per session and collect the lease holder.
	sessionIDs := make([]string, 0, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sid := createSessionViaRouterURL(ctx, t, cluster.RouterURL, accessToken, orgID,
			fmt.Sprintf("dist-session-%d-%d", i, time.Now().UnixNano()))
		sessionIDs = append(sessionIDs, sid)
	}

	holderSet := make(map[int]struct{})
	for i, sessionID := range sessionIDs {
		getSessionViaRouter(ctx, t, cluster.RouterURL, accessToken, orgID, sessionID)

		holder := cluster.LeaseHolder(ctx, t, sessionID)
		require.GreaterOrEqualf(t, holder, 0,
			"different_sessions_distribute: session[%d]=%s: LeaseHolder returned -1 — "+
				"no pod holds the advisory lock after routing",
			i, sessionID)
		holderSet[holder] = struct{}{}
		t.Logf("different_sessions_distribute: session[%d]=%s → pod %d", i, sessionID, holder)
	}

	require.GreaterOrEqualf(t, len(holderSet), 2,
		"different_sessions_distribute: all %d sessions landed on a single pod; "+
			"consistent-hash ring should distribute across at least 2 of the 3 pods — "+
			"if this is reproducible, this may indicate a real ring-balance bug; "+
			"pod distribution: %v",
		sessionCount, holderSet)

	t.Logf("different_sessions_distribute: %d distinct pods served %d sessions ✓",
		len(holderSet), sessionCount)
}

// ---------------------------------------------------------------------------
// Request helpers local to this file
// ---------------------------------------------------------------------------

// createSessionViaRouterURL posts to /api/orgs/{orgID}/sessions through the
// router URL and returns the new session ID.
//
// This is functionally identical to the helper in cluster_smoke_test.go
// (which lives in a different package), but is kept here to avoid a
// cross-package dependency on a test-package function.
func createSessionViaRouterURL(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body := map[string]string{
		"name":         name,
		"goal":         "router consistent-hash test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "createSessionViaRouterURL: marshal body")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", routerURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "createSessionViaRouterURL: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "createSessionViaRouterURL: POST")
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"createSessionViaRouterURL: want 201 Created; body: %s", respBody)

	var s routerSessionRef
	require.NoError(t, json.Unmarshal(respBody, &s),
		"createSessionViaRouterURL: decode response body: %s", respBody)
	require.NotEmpty(t, s.ID, "createSessionViaRouterURL: empty session ID in response")

	return s.ID
}

// getSessionViaRouter issues GET /api/orgs/{orgID}/sessions/{sessionID}
// through the router URL and asserts a 2xx response. The response body is
// discarded — this helper's purpose is to drive a request through the router
// so the portal acquires the advisory lock (routing identity), not to
// validate the response content.
func getSessionViaRouter(
	ctx context.Context,
	t *testing.T,
	routerURL, accessToken, orgID, sessionID string,
) {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", routerURL, orgID, sessionID),
		nil)
	require.NoError(t, err, "getSessionViaRouter: build request")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "getSessionViaRouter: GET")
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Truef(t,
		resp.StatusCode >= 200 && resp.StatusCode < 300,
		"getSessionViaRouter: want 2xx; got %d; body: %s", resp.StatusCode, respBody)
}
