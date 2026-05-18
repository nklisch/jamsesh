// Invariant: when pod 0's process clock is advanced forward (simulating an NTP
// clock jump), lease ownership must remain stable. The Postgres advisory lock
// is connection-scoped, not wall-clock-TTL-scoped — the lock releases only
// when the Postgres connection drops, not when any clock expires.
//
// This test probes the heartbeat path: the portal's runHeartbeat goroutine
// uses time.NewTicker with an interval equal to JAMSESH_LEASE_HEARTBEAT_INTERVAL_S.
// If the process clock is advanced (via /test/clock-advance), the ticker fires
// at an accelerated rate, which shrinks the PingContext timeout window. If the
// PingContext timeout expires before the ping returns, the heartbeat fails and
// the advisory lock is spuriously released, creating a split-brain window.
//
// WARNING — this test is designed to surface a real bug. If the portal's
// runHeartbeat uses time.NewTicker with a timeout equal to the ticker interval,
// advancing the clock WILL cause spurious lease loss. If this test fails
// consistently:
//  1. Park the bug via /agile-workflow:park with title
//     "Clock skew accelerates heartbeat ticker causing spurious lease loss".
//  2. Land the test with t.Skip("<backlog-id>: clock skew causes heartbeat
//     timeout under local-clock-anchored ticker").
//  3. Do NOT change the assertion. The skipped test is the audit trail.
package chaos_test

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

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestCrossPodClockSkew verifies that advancing pod 0's process clock does not
// destabilize lease ownership. The test:
//
//  1. Starts a 2-pod cluster with Router: true and a 2-second heartbeat.
//  2. Acquires leases on two sessions (routed to different or the same pod —
//     the router's consistent-hash distributes them).
//  3. Advances pod 0's clock by 4× the heartbeat interval via
//     /test/clock-advance (the existing clock-skew test mechanism from
//     clockadvance.go).
//  4. Waits long enough for the skewed ticker to have fired several extra
//     times, then checks that both sessions still have lease holders.
//
// If either session loses its holder (holder = -1), the clock skew caused a
// spurious advisory-lock release. That is a real split-brain risk — park the
// bug and t.Skip with the backlog id.
func TestCrossPodClockSkew(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos: long-running, skip under -short")
	}

	requireDocker(t)
	requirePortalImage(t)

	ctx := context.Background()

	// heartbeatS is the lease heartbeat interval in seconds. 2s keeps the
	// test wall-clock cost low (SLO wait is skewSeconds + 2×heartbeat).
	const heartbeatS = 2
	// skewSeconds advances pod 0's clock by 4× the heartbeat. This is
	// enough to cause 4 extra ticker fires in the skewed period — sufficient
	// to trigger a PingContext timeout if the timeout equals the interval.
	const skewSeconds = heartbeatS * 4 // 8 seconds

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true,
		PortalExtraEnv: map[string]string{
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": fmt.Sprintf("%d", heartbeatS),
			"JAMSESH_EMAIL_PROVIDER":             "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":            mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":            strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":             "none",
		},
	})

	// ── Auth + session creation ─────────────────────────────────────────────
	alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh,
		leaseChaosEmail(t, "alice-skew"))
	userID := leaseSkewGetMe(ctx, t, c.Pods[0].URL, alice.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "ClockSkew Org")
	sessionA := createLeaseSkewSession(ctx, t, c.RouterURL, alice.AccessToken, orgID, "skew-session-a")
	sessionB := createLeaseSkewSession(ctx, t, c.RouterURL, alice.AccessToken, orgID, "skew-session-b")

	// ── BEFORE-CHAOS BASELINE ───────────────────────────────────────────────
	// Push to both sessions to trigger lease acquisition. The router's
	// consistent-hash may route both sessions to the same pod or split them.
	pushLeaseSkew(ctx, t, c.RouterURL, orgID, sessionA, userID, alice.AccessToken, "skew-a-baseline")
	pushLeaseSkew(ctx, t, c.RouterURL, orgID, sessionB, userID, alice.AccessToken, "skew-b-baseline")

	// Confirm both sessions have lease holders before injecting clock skew.
	holderA := c.RequireLeaseHolder(ctx, t, sessionA, 10*time.Second)
	holderB := c.RequireLeaseHolder(ctx, t, sessionB, 10*time.Second)
	t.Logf("cross_pod_clock_skew: baseline: sessionA held by pod %d, sessionB held by pod %d",
		holderA, holderB)

	// ── INJECT CLOCK SKEW on pod 0 ─────────────────────────────────────────
	// AdvanceClock POSTs to /test/clock-advance on pod 0, advancing the
	// process-global clock by skewSeconds. The portal must be built with
	// -tags e2etest for this endpoint to exist (the standard make target
	// test-portal-image includes this build tag).
	//
	// Effect on the heartbeat: time.NewTicker(2s) ticks based on the process
	// clock. After the advance, the ticker may fire more rapidly (if the
	// runtime notices the clock jumped) or the PingContext timeout may be
	// measured against the skewed clock. Either way, if the heartbeat timeout
	// fires before the ping returns, the advisory lock is released spuriously.
	c.Pods[0].AdvanceClock(ctx, t, time.Duration(skewSeconds)*time.Second)
	t.Logf("cross_pod_clock_skew: advanced pod 0 clock by %ds", skewSeconds)

	// ── Wait for the skew window to elapse ─────────────────────────────────
	// Wait long enough for the accelerated ticker to have fired skewSeconds /
	// heartbeatS = 4 additional times, plus 2 normal heartbeat periods for
	// the non-skewed pod 1 to detect any lock state change.
	//
	// Wall-clock wait is against the real clock (time.Sleep is not affected
	// by the portal-internal clock advance — that is per-portal-process, not
	// the test process). This is intentional: we want real wall-clock time to
	// elapse so the ticker has enough real time to fire.
	waitDur := time.Duration(skewSeconds+heartbeatS*2) * time.Second
	t.Logf("cross_pod_clock_skew: waiting %v for skew effects to manifest", waitDur)
	time.Sleep(waitDur)

	// ── ASSERT: leases are still held (no spurious loss) ───────────────────
	// LeaseHolder queries Postgres pg_locks directly. If the clock skew
	// caused a spurious advisory-lock release, the lock for the affected
	// session will be gone and LeaseHolder returns -1.
	//
	// ESCAPE HATCH (see file-level comment): if either holder is -1 here,
	// the clock skew caused a real lease loss. DO NOT change the assertion.
	// Park the bug and t.Skip with the backlog id.
	newHolderA := c.LeaseHolder(ctx, t, sessionA)
	newHolderB := c.LeaseHolder(ctx, t, sessionB)

	t.Logf("cross_pod_clock_skew: post-skew: sessionA holder=%d, sessionB holder=%d",
		newHolderA, newHolderB)

	if newHolderA < 0 || newHolderB < 0 {
		// Clock skew caused advisory-lock loss. This is a real split-brain risk:
		// the heartbeat timeout (PingContext) fired before the ping returned,
		// closing the advisory-lock connection and releasing the lock.
		//
		// TO MAINTAINER: if this fires consistently:
		//   1. Park: /agile-workflow:park
		//      Title: "Clock skew accelerates heartbeat ticker causing spurious lease loss"
		//      Severity: Critical
		//      Body: "runHeartbeat uses time.NewTicker(interval) with PingContext
		//             timeout = interval. A process-clock advance shrinks the
		//             effective timeout window, causing spurious lease eviction.
		//             Real split-brain risk under NTP clock jump."
		//   2. Replace this t.Fatalf with:
		//      t.Skip("<backlog-id>: clock skew causes heartbeat timeout under
		//              local-clock-anchored ticker")
		//   3. Do NOT remove this test or change the assertion.
		t.Fatalf(
			"cross_pod_clock_skew: CLOCK SKEW CAUSED LEASE LOSS — "+
				"sessionA holder=%d, sessionB holder=%d after %ds skew on pod 0; "+
				"this is a real split-brain risk: advisory lock was spuriously released "+
				"because the heartbeat PingContext timeout fired before the ping returned. "+
				"The heartbeat ticker (time.NewTicker) is anchored to the local process clock. "+
				"Park the bug as Critical and t.Skip with backlog-id; do NOT change the assertion.",
			newHolderA, newHolderB, skewSeconds)
	}

	// Leases are stable after clock skew: the implementation is robust.
	// This means either:
	//   a) The heartbeat timeout uses a wall-clock-independent mechanism
	//      (e.g. Postgres server-side timeout, not PingContext with a Go timer).
	//   b) The clock advance does not affect the ticker in the way the bug
	//      path predicts (e.g. the runtime doesn't fast-forward the ticker
	//      on a process-clock jump).
	// Either outcome is safe — the lease system is not destabilized by this
	// clock skew magnitude.
	t.Logf(
		"cross_pod_clock_skew: leases stable after %ds clock skew on pod 0 "+
			"(sessionA pod=%d, sessionB pod=%d) — implementation is clock-skew-robust ✓",
		skewSeconds, newHolderA, newHolderB)
}

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

// leaseSkewSessionRef mirrors the minimal create-session JSON response.
type leaseSkewSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// leaseSkewUserRef mirrors the minimal /me JSON response.
type leaseSkewUserRef struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// createLeaseSkewSession creates a session at baseURL and returns its ID.
func createLeaseSkewSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body := map[string]string{
		"name":         name,
		"goal":         "cross-pod clock-skew chaos e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("createLeaseSkewSession: marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("createLeaseSkewSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createLeaseSkewSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createLeaseSkewSession: want 201; got %d; body: %s", resp.StatusCode, respBody)
	}

	var s leaseSkewSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("createLeaseSkewSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("createLeaseSkewSession: empty session ID; body: %s", respBody)
	}
	return s.ID
}

// leaseSkewGetMe calls GET /api/me and returns the caller's user ID.
func leaseSkewGetMe(ctx context.Context, t *testing.T, podURL, accessToken string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("leaseSkewGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("leaseSkewGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("leaseSkewGetMe: want 200; got %d; body: %s", resp.StatusCode, body)
	}

	var me leaseSkewUserRef
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("leaseSkewGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("leaseSkewGetMe: empty user ID; body: %s", body)
	}
	return me.ID
}

// pushLeaseSkew performs a git push to baseURL to trigger lease acquisition.
// Each call uses a unique label in the commit message and filename to guarantee
// a non-empty delta (an identical tree push is a no-op in git).
func pushLeaseSkew(
	ctx context.Context, t *testing.T,
	baseURL, orgID, sessionID, userID, accessToken, label string,
) {
	t.Helper()

	repo := gitclient.Clone(ctx, t, baseURL, orgID, sessionID, userID, accessToken)
	ref := fmt.Sprintf("jam/%s/%s/main", sessionID, userID)
	filename := fmt.Sprintf("clock-skew-%s.md", label)
	repo.Commit(ctx, t, filename, "clock skew chaos content — "+label, "clock-skew: "+label)
	repo.Push(ctx, t, ref)
}
