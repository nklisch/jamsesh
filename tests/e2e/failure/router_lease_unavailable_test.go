// Invariant: when a portal pod returns 503 (lease held elsewhere), the router
// re-dispatches the request transparently — the client sees a single 2xx
// response, never a 503. When all pods return 503, the router stops after
// exactly one retry (two total pod attempts) and surfaces 503 to the client
// within a bounded time.
//
// Two subtests:
//
//  1. transparent_redispatch_on_503 — hold a Postgres advisory lock from the
//     test process so the ring-chosen pod cannot acquire it; verify the router
//     re-dispatches and the client sees a clean 2xx.
//
//  2. bounded_retry_pathology_surfaces_503 — hold advisory locks for all
//     pods; verify the router gives up after a bounded number of attempts
//     and returns 503 within a tight wall-clock window (≤5s).
//
// Advisory lock strategy: the portal acquires pg_try_advisory_lock(hashtext(sessionID)).
// hashtext() returns int4, passed directly as the 64-bit bigint key of the
// single-argument advisory-lock form — there is NO pg_advisory_lock(oid)
// overload, so the test must use the same bare hashtext($1) key, not a ::oid
// cast. Holding that same lock from the test connection makes the portal's
// non-blocking try-lock fail, triggering its 503 path. See
// internal/portal/lease/postgres.go and lifecycle.go in the portalcluster fixture.
package failure_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestRouterLeaseUnavailable exercises the router's 503-retry contract against
// a real 2-pod clustered deployment using advisory-lock injection to induce
// controlled 503 responses from individual pods.
func TestRouterLeaseUnavailable(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	// SKIP: idea-router-e2e-lease-premise. Both subtests try to induce a 503
	// from a portal pod by holding the per-session advisory lock from the test
	// process, then drive a session-scoped REST GET through the router. But the
	// REST GET path (GetSession) reads Postgres directly and never acquires or
	// contends on that advisory lock — only the git/objectstore LifecycleManager
	// does — so a held lock can never make a read GET return 503. As a result
	// `transparent_redispatch_on_503` passes tautologically (the read is always
	// 200, so no re-dispatch is ever exercised) and `bounded_retry` can never
	// observe the all-pods-503 condition it asserts. The router's re-dispatch
	// product code IS correct and is exercised deterministically by the unit test
	// Test503RetrySucceeds in internal/router/proxy (buffered first attempt,
	// retry replaces a leaked 503, bounded to one retry). To make these e2e tests
	// valid they must be re-anchored to a path that genuinely 503s under lock
	// contention (git push/clone through the router), tracked in the backlog item.
	// Do NOT relax the 2xx/503 assertions to force a green — that would lie about
	// the re-dispatch invariant.
	t.Skip("router 503 re-dispatch failure tests are blocked on idea-router-e2e-lease-premise: " +
		"a held advisory lock cannot force a 503 on the REST GET path (the lease is git-only), so " +
		"transparent_redispatch is tautological and bounded_retry cannot reach all-pods-503; the " +
		"re-dispatch product fix is covered by the proxy unit test Test503RetrySucceeds " +
		"(see .work/backlog/idea-router-e2e-lease-premise.md)")

	t.Run("transparent_redispatch_on_503", testTransparentRedispatchOn503)
	t.Run("bounded_retry_pathology_surfaces_503", testBoundedRetryPathologySurfaces503)
}

// testTransparentRedispatchOn503 holds a Postgres advisory lock from the test
// process for the session's ring-chosen pod, then sends a session-scoped
// request through the router. The router must re-dispatch to the other pod
// and return 2xx to the client — the 503 from the primary pod must never
// escape to the client.
func testTransparentRedispatchOn503(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
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
	require.NotEmpty(t, cluster.RouterURL, "transparent_redispatch: router URL must be set")

	// Auth: sign in against pod 0 (all pods share Postgres — tokens are visible
	// cluster-wide).
	pod0 := cluster.Pods[0]
	userEmail := fmt.Sprintf("redispatch-%d@example.com", time.Now().UnixNano())
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Redispatch Org")

	// Create the session via the router so the ring can learn its session ID
	// before we try to trigger a 503. The session creation itself acquires the
	// advisory lock on whichever pod wins — that's fine; we will hold a second
	// test-side lock to compete with it.
	sessionID := createSessionViaRouterHelper(ctx, t, cluster.RouterURL, pair.AccessToken, orgID)
	t.Logf("transparent_redispatch: session %s created", sessionID)

	// ── Hold the advisory lock from the test process ─────────────────────────
	// We open a dedicated DB connection that acquires pg_advisory_lock (blocking,
	// not try) for this session ID. When the portal's non-blocking
	// pg_try_advisory_lock call races against ours it will fail and return 503.
	//
	// Note: pg_advisory_lock blocks if another session holds it. We need to
	// release the portal's lock first. We do this by waiting briefly for the
	// portal to release (after the session creation response). The portal holds
	// the lock only for the duration of active session operations; it releases
	// after the request completes in single-request mode. In clustered mode the
	// lease is held persistently. We use pg_advisory_unlock_all in cleanup and
	// hold from a separate connection that the portal will have to compete with.
	//
	// The correct approach: hold the lock from a fresh connection before the
	// next request hits the ring-assigned pod. Since we need the lock on the
	// *same* key the portal uses, we acquire it here and the portal's
	// pg_try_advisory_lock will fail for the holder pod.
	lockDB, err := sql.Open("postgres", pg.DSN)
	require.NoError(t, err, "transparent_redispatch: open lock DB")
	defer lockDB.Close()

	// Acquire the advisory lock for this session from the test side.
	// pg_advisory_lock blocks until acquired; since no pod currently holds it
	// (session was just created and request completed), we should get it immediately.
	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", sessionID)
	require.NoError(t, err, "transparent_redispatch: acquire advisory lock from test side")

	t.Logf("transparent_redispatch: advisory lock held by test process; sending session-scoped request via router")

	// ── Send a session-scoped request through the router ────────────────────
	// GET /api/orgs/{orgID}/sessions/{sessionID} is a safe read-only probe
	// that still scopes to the session (the router extracts the session ID from
	// the URL path and routes accordingly).
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", cluster.RouterURL, orgID, sessionID),
		nil)
	require.NoError(t, err, "transparent_redispatch: build request")
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	require.NoError(t, err, "transparent_redispatch: GET session via router")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	// Invariant: the client MUST see 2xx — the router re-dispatched to the
	// other pod. A 503 here means the router leaked the backend 503 to the
	// client, which is a routing correctness bug.
	require.Truef(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"transparent_redispatch: client must receive 2xx (router re-dispatch invariant); "+
			"got %d after %v — if 503: the router is NOT re-dispatching; park as Important bug. "+
			"body: %s", resp.StatusCode, elapsed, respBody)

	// Wall-clock bound: re-dispatch should complete well within 3s.
	require.LessOrEqualf(t, elapsed, 3*time.Second,
		"transparent_redispatch: response must arrive within 3s; took %v", elapsed)

	t.Logf("transparent_redispatch: got %d in %v (re-dispatch succeeded)", resp.StatusCode, elapsed)

	// ── Release the advisory lock ─────────────────────────────────────────────
	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
	require.NoError(t, err, "transparent_redispatch: release advisory lock")
	t.Logf("transparent_redispatch: advisory lock released")

	// ── Verify subsequent request succeeds without retry ─────────────────────
	// After releasing the lock the portal can re-acquire the lease. The next
	// request should succeed cleanly (ring or hint-cache routes correctly).
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", cluster.RouterURL, orgID, sessionID),
		nil)
	require.NoError(t, err, "transparent_redispatch: build follow-up request")
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err, "transparent_redispatch: follow-up GET session via router")
	defer resp2.Body.Close()
	respBody2, _ := io.ReadAll(resp2.Body)

	require.Truef(t, resp2.StatusCode >= 200 && resp2.StatusCode < 300,
		"transparent_redispatch: follow-up request after lock release must be 2xx; got %d, body: %s",
		resp2.StatusCode, respBody2)
	t.Logf("transparent_redispatch: follow-up request got %d (post-release health confirmed)", resp2.StatusCode)
}

// testBoundedRetryPathologySurfaces503 holds advisory locks for the session
// from the test side so neither pod can acquire it, then verifies the router
// stops retrying after exactly one retry (two total pod attempts) and returns
// 503 to the client within a bounded wall-clock window.
//
// Bounded-retry design (from proxy.go): the router calls Ring.GetNext on the
// first 503 for exactly one retry attempt. With 2 pods and both returning
// 503, the client receives 503 after the two pod attempts. The test guards
// the no-infinite-retry invariant by measuring wall-clock time.
func testBoundedRetryPathologySurfaces503(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
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
	require.NotEmpty(t, cluster.RouterURL, "bounded_retry: router URL must be set")

	// Auth: sign in and create a session via the router.
	pod0 := cluster.Pods[0]
	userEmail := fmt.Sprintf("bounded-retry-%d@example.com", time.Now().UnixNano())
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Bounded Retry Org")
	sessionID := createSessionViaRouterHelper(ctx, t, cluster.RouterURL, pair.AccessToken, orgID)
	t.Logf("bounded_retry: session %s created", sessionID)

	// ── Hold the advisory lock so ALL pods see contention ───────────────────
	// We use a single DB connection that holds pg_advisory_lock for the session.
	// This means every portal pod's pg_try_advisory_lock call will fail, causing
	// each pod to return 503. The router will try pod 0, get 503, retry pod 1,
	// get 503 again, and propagate 503 to the client.
	//
	// A single connection holding the lock is sufficient because pg_advisory_lock
	// is session-scoped: any other DB session trying pg_try_advisory_lock on the
	// same key will get false regardless of which portal pod issues the query.
	lockDB, err := sql.Open("postgres", pg.DSN)
	require.NoError(t, err, "bounded_retry: open lock DB")
	defer lockDB.Close()

	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", sessionID)
	require.NoError(t, err, "bounded_retry: acquire advisory lock to block all pods")
	t.Logf("bounded_retry: advisory lock held; both pods will return 503")

	// ── Send the session-scoped request and measure wall-clock time ──────────
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", cluster.RouterURL, orgID, sessionID),
		nil)
	require.NoError(t, err, "bounded_retry: build request")
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	require.NoError(t, err, "bounded_retry: GET session via router (must not hang — connection error means infinite retry)")
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck // drain body

	// Invariant 1: the client must receive 503 — not 2xx, not a hang.
	// If the client gets 2xx here, the advisory lock didn't block the portal
	// as expected (test setup issue). If the client hangs past the bounded-time
	// window, the router has infinite-retry pathology — park as Important.
	require.Equalf(t, http.StatusServiceUnavailable, resp.StatusCode,
		"bounded_retry: client must receive 503 when all pods are unavailable; got %d after %v",
		resp.StatusCode, elapsed)

	// Invariant 2: the 503 must arrive within 5s.
	// The router makes exactly 2 pod attempts (initial + one retry from GetNext).
	// Each pod attempt should fail fast (pg_try_advisory_lock is non-blocking).
	// If elapsed > 5s, the router is hanging on retries — an infinite-retry bug.
	require.LessOrEqualf(t, elapsed, 5*time.Second,
		"bounded_retry: router must give up within 5s; took %v — "+
			"if this hangs, the router has infinite-retry pathology (park as Important)", elapsed)

	t.Logf("bounded_retry: got 503 in %v (bounded-retry invariant satisfied)", elapsed)

	// ── Release both locks ───────────────────────────────────────────────────
	_, err = lockDB.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
	require.NoError(t, err, "bounded_retry: release advisory lock")
	t.Logf("bounded_retry: advisory lock released")
}

// ---------------------------------------------------------------------------
// Per-file helpers
// ---------------------------------------------------------------------------

// routerSessionRef is the minimal subset of the session-creation response.
type routerSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createSessionViaRouterHelper posts to /api/orgs/{orgID}/sessions through the
// router URL and returns the new session ID. Named with "Helper" suffix to
// avoid conflict with the shared createSessionViaRouter in cluster_smoke_test.go
// (which lives in a different package — scaffolding_test — so there is no
// symbol collision, but the explicit suffix keeps intent clear).
func createSessionViaRouterHelper(ctx context.Context, t *testing.T, routerURL, accessToken, orgID string) string {
	t.Helper()
	body := map[string]string{
		"name":         "router-503-test",
		"goal":         "503 re-dispatch e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "createSessionViaRouterHelper: marshal body")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", routerURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "createSessionViaRouterHelper: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "createSessionViaRouterHelper: POST")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"createSessionViaRouterHelper: want 201; body: %s", respBody)

	var s routerSessionRef
	require.NoError(t, json.Unmarshal(respBody, &s), "createSessionViaRouterHelper: decode response")
	require.NotEmpty(t, s.ID, "createSessionViaRouterHelper: empty session ID")
	return s.ID
}
