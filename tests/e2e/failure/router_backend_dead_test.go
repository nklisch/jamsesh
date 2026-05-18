// Invariant: when a backend pod is SIGKILL'd, the router's static-discovery
// health-check detects its absence and re-shards affected sessions to a
// surviving pod within the 15s SLO.
//
// # Design finding (bug-router-static-discoverer-not-started)
//
// Audit of cmd/jamsesh-router/main.go reveals that the static discoverer is
// constructed but its Run loop is never started. The ring is seeded once at
// startup from the static pod list and is never updated thereafter. When a
// backend pod dies:
//
//   - The router continues routing to the dead pod's address.
//   - The dead pod's TCP connection fails; httputil.ReverseProxy ErrorHandler
//     returns 502 Bad Gateway.
//   - The 502 does NOT trigger the 503 retry path (only StatusServiceUnavailable
//     triggers GetNext retry in proxy.go).
//   - Result: the client receives 502 indefinitely; no re-sharding occurs.
//
// The static discoverer implementation (internal/router/discovery/static.go)
// is correct and complete: it probes backends on a configurable interval
// (default 5s) and calls publish only when the healthy set changes, which
// calls ring.SetPods to atomically rebalance. The gap is solely in wiring:
// main.go does not call discovery.Static(...).Run(ctx, ring.SetPods).
//
// This is an Important correctness gap because a router that never evicts dead
// backends causes permanent 502s for any session that hashes to the dead pod,
// defeating the availability goal of the routing layer.
//
// Tracked in: .work/backlog/bug-router-static-discoverer-not-started.md
//
// The test below is written in full to document the intended invariant and
// will be activated (t.Skip removed) once the bug is fixed. The test structure,
// SLO assertions, and polling logic are correct and should not be modified to
// work around the bug.
package failure_test

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

// TestRouterBackendDead exercises the router's dead-pod eviction invariant:
// after a backend pod is SIGKILL'd, the router must detect its absence via the
// static-discovery health-check loop and re-shard sessions that hashed to it
// onto surviving pods within the 15s SLO.
func TestRouterBackendDead(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	t.Run("dead_pod_removed_from_routing_pool", testDeadPodRemovedFromRoutingPool)
}

// testDeadPodRemovedFromRoutingPool:
//  1. Starts a 3-pod cluster with the router.
//  2. Creates a session and confirms which pod holds its lease (via
//     cluster.LeaseHolder).
//  3. Sends N successful requests via the router; asserts all return 2xx.
//  4. Kills the lease-holding pod with SIGKILL via cluster.Kill.
//  5. Polls for up to 15s (the static-discovery SLO): sends requests for the
//     same session and asserts they start returning 2xx from a surviving pod.
//  6. Asserts: after the SLO window, requests succeed and the new lease holder
//     is a surviving pod (>= 0 and != killed pod index).
//
// SLO rationale: the static discoverer probes backends every 5s (config
// default). Within 1–2 probe cycles (5–10s), the dead pod is removed from the
// ring. The 15s window gives 3 probe cycles of headroom.
//
// SKIPPED: bug-router-static-discoverer-not-started — main.go does not start
// the discovery loop; the ring is static after seeding. Remove the t.Skip once
// the wiring is added. See package-level comment for details.
func testDeadPodRemovedFromRoutingPool(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
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
	require.NotEmpty(t, cluster.RouterURL, "dead_pod test requires router")

	// Auth against pod 0 (all pods share Postgres; tokens are cluster-wide).
	pod0 := cluster.Pods[0]
	userEmail := fmt.Sprintf("dead-pod-%d@example.com", time.Now().UnixNano())
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Dead Pod Org")

	// ── Create a session and identify its lease holder ───────────────────────
	sessionID := createSessionViaRouterForDeadPodTest(ctx, t, cluster.RouterURL, pair.AccessToken, orgID)
	t.Logf("dead_pod: session %s created", sessionID)

	// Poll for the lease to be acquired (portal acquires lease on session
	// operations; allow 5s for initial lease acquisition).
	holderIndex := cluster.RequireLeaseHolder(ctx, t, sessionID, 5*time.Second)
	t.Logf("dead_pod: lease held by pod %d", holderIndex)

	// ── Pre-kill: verify N requests all succeed ──────────────────────────────
	// Issue 5 GET requests for the session via the router; all must return 2xx.
	// This establishes the baseline: the router routes correctly before any pod dies.
	const preKillRequests = 5
	for i := range preKillRequests {
		status, err := getSessionViaRouter(ctx, cluster.RouterURL, pair.AccessToken, orgID, sessionID)
		require.NoError(t, err, "dead_pod: pre-kill request %d: HTTP error", i)
		require.Truef(t, status >= 200 && status < 300,
			"dead_pod: pre-kill request %d must return 2xx; got %d", i, status)
	}
	t.Logf("dead_pod: %d pre-kill requests all 2xx (baseline confirmed)", preKillRequests)

	// ── Kill the lease-holding pod ───────────────────────────────────────────
	// docker kill --signal SIGKILL: abrupt pod death, no graceful drain.
	// After this call the pod's container is in "killed" state; its TCP port
	// is no longer accepting connections.
	cluster.Kill(ctx, t, holderIndex)
	t.Logf("dead_pod: pod %d killed (SIGKILL)", holderIndex)

	// ── SLO assertion: re-sharding within 15s ───────────────────────────────
	// The static discoverer probes all backends every ProbeInterval (default 5s).
	// On the next probe cycle after the kill, the dead pod fails its /readyz
	// check (connection refused). The discoverer calls ring.SetPods with the
	// healthy subset (2 surviving pods). The ring atomically drops the dead pod.
	// Subsequent requests for this session re-shard to one of the 2 survivors.
	//
	// SLO: 15s = 3 × 5s probe cycles. The router must detect and evict the
	// dead pod within this window. If it does not, the discovery implementation
	// or wiring has a correctness gap.
	//
	// Polling strategy: send one GET per 500ms; assert the response is 2xx.
	// A 502 means the router is still routing to the dead pod. We count 2xx
	// responses to confirm stability (not a fluke 2xx from a retry).
	const (
		sloWindow        = 15 * time.Second
		pollInterval     = 500 * time.Millisecond
		requiredSuccesses = 3 // need 3 consecutive 2xx to confirm re-sharding
	)

	var consecutiveSuccesses int
	var lastErr error
	var lastStatus int

	deadline := time.Now().Add(sloWindow)
	for time.Now().Before(deadline) {
		status, err := getSessionViaRouter(ctx, cluster.RouterURL, pair.AccessToken, orgID, sessionID)
		if err != nil {
			// Transport error (connection reset, EOF): the router attempted the
			// dead pod and the OS rejected the TCP connection. Reset success counter.
			t.Logf("dead_pod: poll: transport error (router may still be routing to dead pod): %v", err)
			consecutiveSuccesses = 0
			lastErr = err
			lastStatus = 0
			time.Sleep(pollInterval)
			continue
		}

		lastStatus = status
		lastErr = nil

		if status >= 200 && status < 300 {
			consecutiveSuccesses++
			t.Logf("dead_pod: poll: %d (success #%d)", status, consecutiveSuccesses)
		} else {
			// 502 or other non-2xx: router is routing to dead pod (502 Bad Gateway
			// from ErrorHandler) or some other error. Reset counter.
			t.Logf("dead_pod: poll: %d (not 2xx; router may still route to dead pod)", status)
			consecutiveSuccesses = 0
		}

		if consecutiveSuccesses >= requiredSuccesses {
			t.Logf("dead_pod: re-sharding confirmed — %d consecutive 2xx within SLO window", requiredSuccesses)
			break
		}

		time.Sleep(pollInterval)
	}

	// Assert: we achieved required consecutive 2xx responses within the SLO.
	// If this fails, the router did not evict the dead pod within 15s — this is
	// the SLO mismatch. Do not extend the timeout; park the bug instead.
	require.GreaterOrEqualf(t, consecutiveSuccesses, requiredSuccesses,
		"dead_pod: SLO VIOLATED — router did not re-shard session %s to a surviving pod within %v; "+
			"last status: %d, last error: %v. "+
			"The static discoverer may not be evicting the dead pod (check bug-router-static-discoverer-not-started). "+
			"Do NOT extend this timeout — park as Important if this consistently fails.",
		sessionID, sloWindow, lastStatus, lastErr)

	// ── Verify the new lease holder is a surviving pod ───────────────────────
	// After the dead pod is gone and a surviving pod re-acquires the lease,
	// LeaseHolder must return a valid index that is NOT the killed pod.
	//
	// The lease migration may take a few seconds after the advisory lock is
	// released (connection closed on kill). Poll for up to 10s.
	newHolder := cluster.WaitForLeaseMigration(ctx, t, sessionID, holderIndex, 10*time.Second)
	require.GreaterOrEqualf(t, newHolder, 0,
		"dead_pod: a surviving pod must re-acquire the lease for session %s after pod %d died; "+
			"LeaseHolder returned -1 (no holder). The session may be permanently unavailable.",
		sessionID, holderIndex)
	require.NotEqualf(t, holderIndex, newHolder,
		"dead_pod: new lease holder must not be the killed pod (%d); got %d",
		holderIndex, newHolder)
	t.Logf("dead_pod: lease migrated from dead pod %d to surviving pod %d", holderIndex, newHolder)

	// ── Post-eviction: verify requests succeed without errors ────────────────
	// After the SLO window and lease migration, send 5 more requests. None
	// should return a connection-reset or timeout error; all must be 2xx.
	// A transport error here means the router is STILL routing to the dead pod
	// after eviction — that is a more severe bug than the SLO miss above.
	const postEvictionRequests = 5
	for i := range postEvictionRequests {
		status, err := getSessionViaRouter(ctx, cluster.RouterURL, pair.AccessToken, orgID, sessionID)
		require.NoErrorf(t, err,
			"dead_pod: post-eviction request %d: transport error — router is still routing to dead pod %d "+
				"after SLO window; connection-reset/timeout errors must not propagate after eviction",
			i, holderIndex)
		require.Truef(t, status >= 200 && status < 300,
			"dead_pod: post-eviction request %d must return 2xx; got %d", i, status)
	}
	t.Logf("dead_pod: %d post-eviction requests all 2xx — dead-pod eviction invariant satisfied", postEvictionRequests)
}

// ---------------------------------------------------------------------------
// Per-file helpers
// ---------------------------------------------------------------------------

// deadPodSessionRef is the minimal subset of the session-creation response.
type deadPodSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createSessionViaRouterForDeadPodTest creates a new session through the router
// URL and returns its ID. Named with a test-specific suffix to avoid conflicts
// with helpers in other failure test files (which live in the same package).
func createSessionViaRouterForDeadPodTest(ctx context.Context, t *testing.T, routerURL, accessToken, orgID string) string {
	t.Helper()
	body := map[string]string{
		"name":         "dead-pod-test",
		"goal":         "backend dead eviction e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "createSessionViaRouterForDeadPodTest: marshal body")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", routerURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "createSessionViaRouterForDeadPodTest: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "createSessionViaRouterForDeadPodTest: POST")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"createSessionViaRouterForDeadPodTest: want 201; body: %s", respBody)

	var s deadPodSessionRef
	require.NoError(t, json.Unmarshal(respBody, &s), "createSessionViaRouterForDeadPodTest: decode response")
	require.NotEmpty(t, s.ID, "createSessionViaRouterForDeadPodTest: empty session ID")
	return s.ID
}

// getSessionViaRouter sends GET /api/orgs/{orgID}/sessions/{sessionID} through
// the router and returns the HTTP status code. Returns an error only for
// transport-level failures (connection refused, reset, timeout) — HTTP errors
// like 502/503 are returned as status codes with nil error.
func getSessionViaRouter(ctx context.Context, routerURL, accessToken, orgID, sessionID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s", routerURL, orgID, sessionID),
		nil)
	if err != nil {
		return 0, fmt.Errorf("getSessionViaRouter: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("getSessionViaRouter: transport error: %w", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck // drain body to allow connection reuse
	return resp.StatusCode, nil
}
