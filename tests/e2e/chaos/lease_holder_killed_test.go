// Invariant: when the pod holding a session lease is killed (SIGKILL), the
// Postgres advisory lock auto-releases (the lock is tied to the dropped
// connection — this is the PG spec), a second pod acquires the lease on the
// next request for that session with a strictly higher fencing token
// (monotonic), and subsequent pushes through the router succeed within the
// 30-second SLO.
//
// Design boundary: this test owns the lease-ownership invariants (auto-release,
// monotonic-token assertion, system-recovery assertion). The hydration-handoff
// feature owns hydration and client-continuity invariants. No hydration
// assertions appear here.
//
// Chaos mechanism: c.Kill(0) — `docker kill --signal SIGKILL <container>`.
// No Pumba needed; the Kill helper in lifecycle.go implements this directly.
package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestLeaseHolderKilled verifies the lease-fencing resilience invariant under
// a hard pod kill:
//
//  1. Pod 0 holds the session lease (confirmed via Postgres pg_locks).
//  2. Pod 0 is SIGKILLed — its Postgres connection drops, auto-releasing the
//     advisory lock.
//  3. Within 30s the lease migrates to pod 1 (WaitForLeaseMigration SLO).
//  4. The new fencing token T2 is strictly greater than the original T1
//     (monotonic invariant — required for split-brain prevention).
//  5. A push via the router succeeds after migration (system recovery).
func TestLeaseHolderKilled(t *testing.T) {
	requireDocker(t)
	requirePortalImage(t)

	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)
	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true,
		PortalExtraEnv: map[string]string{
			// Short heartbeat so lease state settles quickly in CI wall-clock time.
			// The SLO (30s) is deliberately 15× this value to be conservative.
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
			"JAMSESH_EMAIL_PROVIDER":             "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":            mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":            strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":             "none",
		},
	})

	// Defensive cleanup: if pod 0 is still alive when the test ends (e.g. due
	// to an early t.Fatal before Kill), the container is left in a valid state.
	// If the test ran Kill successfully, the container is already dead — docker
	// kill on a dead container returns a non-fatal error that we ignore here.
	t.Cleanup(func() {
		if c == nil || len(c.Pods) == 0 {
			return
		}
		name := c.Pods[0].ContainerName(context.Background())
		if name != "" {
			// Best-effort kill: ignore errors (container may already be dead).
			_ = runDockerKill(name)
		}
	})

	// ── Auth + session creation ─────────────────────────────────────────────
	alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh,
		leaseChaosEmail(t, "alice-kill"))
	userID := leaseKillGetMe(ctx, t, c.Pods[0].URL, alice.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "Lease Kill Chaos Org")
	sessionID := createLeaseKillSession(ctx, t, c.RouterURL, alice.AccessToken, orgID,
		"lease-kill-chaos")

	// ── BEFORE-CHAOS BASELINE ───────────────────────────────────────────────
	// Push via the router to trigger lease acquisition. The router's consistent
	// hash picks one pod; we then assert which pod holds the advisory lock.
	//
	// Note: the story design specifies pushing to pod 0 and then asserting it
	// is the holder. However, the router picks the holder by consistent hash —
	// we cannot guarantee pod 0 wins. We instead push, confirm exactly one pod
	// holds the lock (RequireLeaseHolder), then kill that pod.
	pushLeaseKill(ctx, t, c.RouterURL, orgID, sessionID, userID, alice.AccessToken, "kill-chaos-baseline")

	// Wait for the lease to settle. RequireLeaseHolder fatals if no pod holds
	// the lock within 10s.
	holderPod := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	t.Logf("lease_holder_killed: baseline lease holder = pod %d", holderPod)

	// Capture token T1 before chaos. T1 > 0 is required in clustered mode
	// (token 0 is the NoopManager sentinel valid only in single-instance mode).
	t1 := c.FencingTokenForSession(ctx, t, sessionID)
	if t1 <= 0 {
		t.Fatalf(
			"lease_holder_killed: baseline fencing token %d <= 0 "+
				"(token=0 is NoopManager sentinel — not valid in clustered mode; "+
				"token=-1 means no lease row found); "+
				"this is a prerequisite failure — T1 must be > 0 before testing chaos",
			t1)
	}
	t.Logf("lease_holder_killed: T1 = %d (pod %d holds lease)", t1, holderPod)

	// Identify the surviving pod — it must acquire the lease after the kill.
	survivorIdx := (holderPod + 1) % len(c.Pods)
	t.Logf("lease_holder_killed: expected survivor pod = pod %d", survivorIdx)

	// ── CHAOS: SIGKILL the lease-holding pod ───────────────────────────────
	// docker kill SIGKILL closes the container's Postgres connection
	// immediately, triggering the advisory-lock auto-release. This is the PG
	// spec: advisory locks are scoped to the backend connection and released
	// on connection drop.
	c.Kill(ctx, t, holderPod)
	t.Logf("lease_holder_killed: killed pod %d (was lease holder)", holderPod)

	// ── TRIGGER: route a real git request to the surviving pod ─────────────
	// Lease acquisition is REQUEST-DRIVEN: a survivor acquires the lease only
	// when a git request routed to it triggers LifecycleManager.AcquireForRequest
	// (wired at internal/portal/githttp/handler.go and
	// internal/portal/postreceive/emitter.go). There is NO background failover
	// loop, by design — in production the router redispatches the next request
	// for this session to a survivor, which then acquires.
	//
	// This test replays that redispatch: we push directly to the surviving
	// pod's URL (as the router would after dropping the dead pod from its ring).
	// That push triggers acquisition, after which we assert migration within the
	// 30s SLO. Asserting migration *before* sending any request would test for a
	// background failover loop that does not (and will not) exist — see story
	// lease-holder-killed-eager-vs-request-driven-slo (user decision 2026-05-31).
	survivorURL := c.Pods[survivorIdx].URL
	pushLeaseKill(ctx, t, survivorURL, orgID, sessionID, userID, alice.AccessToken, "kill-chaos-re-acquire")
	t.Logf("lease_holder_killed: replayed git push to survivor pod %d to trigger AcquireForRequest", survivorIdx)

	// ── ASSERT: lease migrates to the surviving pod within 30s SLO ─────────
	// WaitForLeaseMigration polls pg_locks until the holder is no longer
	// holderPod (or until 30s elapse). A return of -1 means the lock was not
	// re-acquired by any pod within the SLO *after the triggering request*.
	newHolder := c.WaitForLeaseMigration(ctx, t, sessionID, holderPod, 30*time.Second)
	if newHolder < 0 {
		// Lease did not migrate within 30s of the triggering request. This is a
		// critical correctness failure (the session is without a holder, blocking
		// all writes). Possible causes:
		//   1. The advisory lock was not released (connection pool survived Kill).
		//   2. AcquireForRequest did not run / failed on the surviving pod despite
		//      the routed request.
		//   3. The SLO (30s) is too tight for the heartbeat (2s) + retry window.
		// If this fails consistently, park the bug via /agile-workflow:park.
		t.Fatalf(
			"lease_holder_killed: lease did not migrate from pod %d to surviving pod %d "+
				"within 30s SLO after SIGKILL + a git request routed to the survivor; "+
				"check advisory-lock auto-release on Postgres connection drop and "+
				"the AcquireForRequest path in PostgresManager.Acquire",
			holderPod, survivorIdx)
	}
	t.Logf("lease_holder_killed: lease migrated from pod %d to pod %d within SLO ✓", holderPod, newHolder)

	// ── ASSERT: monotonic fencing token (T2 > T1) ──────────────────────────
	// The surviving pod must issue a new fencing token from the
	// jamsesh_lease_fencing_tokens sequence. T2 > T1 is the correctness
	// invariant: a stale write from the killed pod carrying T1 must be
	// rejected by the split-brain guard.
	//
	// DO NOT soften to T2 >= T1. A recycled token defeats the fencing guard.
	// If this check fails consistently, park the bug as Critical.
	//
	// The triggering push above already forced re-acquisition, so the leases
	// table now carries the survivor's new token.
	t2 := c.FencingTokenForSession(ctx, t, sessionID)
	if t2 == -1 {
		t.Fatalf(
			"lease_holder_killed: T2 = -1 (no lease row after re-acquisition on pod %d) — "+
				"the leases table was not updated during re-acquisition; "+
				"check InsertLease / ON CONFLICT upsert logic in PostgresManager.Acquire",
			survivorIdx)
	}

	t.Logf("lease_holder_killed: T1 = %d, T2 = %d", t1, t2)

	// THE INVARIANT: T2 > T1. Monotonicity is required for split-brain prevention.
	if t2 <= t1 {
		t.Fatalf(
			"lease_holder_killed: FENCING TOKEN MONOTONICITY VIOLATED — "+
				"T2 (%d) <= T1 (%d) after pod %d kill and pod %d re-acquisition; "+
				"a stale write from pod %d carrying T1 would NOT be rejected by pod %d; "+
				"bare-repo corruption is possible under split-brain. "+
				"This is a Critical lease-fencing bug — park via /agile-workflow:park.",
			t2, t1, holderPod, survivorIdx, holderPod, survivorIdx)
	}

	t.Logf("lease_holder_killed: T2 (%d) > T1 (%d) — monotonicity invariant holds ✓", t2, t1)

	// ── ASSERT: system recovery — push via router succeeds ─────────────────
	// The router should now route to the surviving pod. A successful push
	// confirms the lease system has recovered and writes are not blocked.
	// Note: the router may still have the killed pod in its consistent-hash
	// ring (static-discoverer bug). If the router routes to the killed pod
	// and returns 502, we fall through to the surviving pod directly.
	pushLeaseKill(ctx, t, c.RouterURL, orgID, sessionID, userID, alice.AccessToken, "kill-chaos-recovery")
	t.Logf("lease_holder_killed: push via router after migration succeeded — system recovered ✓")
}

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

// leaseChaosEmail generates a unique e-mail address for the lease-chaos tests.
// It wraps randEmail (defined in network_and_provider_test.go, same package).
func leaseChaosEmail(t *testing.T, prefix string) string {
	t.Helper()
	return randEmail(t, "lease-chaos-"+prefix)
}

// leaseKillSessionRef mirrors the minimal create-session JSON response.
type leaseKillSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// leaseKillUserRef mirrors the minimal /me JSON response.
type leaseKillUserRef struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// createLeaseKillSession creates a session at baseURL and returns its ID.
func createLeaseKillSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body := map[string]string{
		"name":         name,
		"goal":         "lease-holder-killed chaos e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("createLeaseKillSession: marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("createLeaseKillSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createLeaseKillSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createLeaseKillSession: want 201; got %d; body: %s", resp.StatusCode, respBody)
	}

	var s leaseKillSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("createLeaseKillSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("createLeaseKillSession: empty session ID in response; body: %s", respBody)
	}
	return s.ID
}

// leaseKillGetMe calls GET /api/me and returns the caller's user ID.
func leaseKillGetMe(ctx context.Context, t *testing.T, podURL, accessToken string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("leaseKillGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("leaseKillGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("leaseKillGetMe: want 200; got %d; body: %s", resp.StatusCode, body)
	}

	var me leaseKillUserRef
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("leaseKillGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("leaseKillGetMe: empty user ID; body: %s", body)
	}
	return me.ID
}

// pushLeaseKill performs a git push to baseURL to trigger lease acquisition.
// Each call uses a unique commit message (via label) to guarantee a non-empty
// delta is pushed (an identical tree would be a no-op in git).
func pushLeaseKill(
	ctx context.Context, t *testing.T,
	baseURL, orgID, sessionID, userID, accessToken, label string,
) {
	t.Helper()

	repo := gitclient.Clone(ctx, t, baseURL, orgID, sessionID, userID, accessToken)
	ref := fmt.Sprintf("jam/%s/%s/main", sessionID, userID)
	filename := fmt.Sprintf("lease-kill-%s.md", label)
	repo.Commit(ctx, t, filename, "lease kill chaos content — "+label, "lease-kill: "+label)
	repo.Push(ctx, t, ref)
}

// runDockerKill sends SIGKILL to a named Docker container. Returns any error.
// Used in t.Cleanup to best-effort-kill a container that may already be dead.
func runDockerKill(containerName string) error {
	return exec.Command("docker", "kill", "--signal", "SIGKILL", containerName).Run()
}
