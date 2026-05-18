// Invariant: when the portal→MinIO path has a transient latency spike during
// pod B's hydration, hydration eventually completes within an extended SLO
// (45s). No commits acknowledged before the chaos window are lost.
//
// Toxiproxy is interposed between all portal pods and MinIO. Both the push-
// sync path (pod A flushing commits before drain) and the hydration-fetch path
// (pod B pulling pack files) route through the proxy, making the test a
// faithful simulation of a flaky S3 connection during session migration.
//
// Design boundary: this test owns the user-visible handoff outcome under
// object-storage chaos. Lease-ownership invariants (auto-release, monotonic
// token) are the domain of lease_holder_killed_test.go. No fencing-token
// assertions appear here.
package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
)

const (
	// stosChaosProxyName is the Toxiproxy proxy name for the portal→MinIO path.
	stosChaosProxyName = "portal-minio"
	// stosChaosProxyPort is the port Toxiproxy listens on inside the Docker
	// network for the MinIO proxy. Portal containers connect to tp.ContainerIP:9101.
	stosChaosProxyPort = 9101
	// stosChaosProxyListen is the Toxiproxy internal listen address.
	stosChaosProxyListen = "0.0.0.0:9101"
	// stosChaosLatencyToxicName identifies the latency toxic so we can remove it.
	stosChaosLatencyToxicName = "hydration-chaos-latency"
	// stosChaosLatencyMs is the latency injected per packet on the
	// portal→MinIO path during hydration. 4 000ms × 8 workers gives ~5-10s
	// of hydration time for O(10) pack objects — well within the 45s SLO.
	stosChaosLatencyMs = 4000
)

// TestHandoffUnderObjectStorageChaos verifies that a handoff completes within
// a 45-second SLO and preserves all pre-chaos acked commits despite a 4-second
// Toxiproxy latency spike on the portal→MinIO path during pod B's hydration.
//
// Sequence:
//  1. Spin up Toxiproxy in front of MinIO; both portal pods route through it.
//  2. Push 5 commits via pod 0 (no toxic yet — baseline RPO=0 holds).
//  3. Inject 4 000ms Toxiproxy latency on portal-minio.
//  4. Graceful-drain pod 0 — pod 0 may be slow to flush, and pod 1 hydrates
//     slowly through the toxic.
//  5. WaitForHydration(pod 1, 45s) — extended SLO for slow hydration.
//  6. Remove the toxic (simulating transient chaos window closing).
//  7. Push commit 6 via pod 1 directly; assert it succeeds.
//  8. All 5 pre-chaos acked commit SHAs must be reachable ancestors of pod 1's
//     current tip (no data loss).
//  9. MinIO bucket has objects for the session.
func TestHandoffUnderObjectStorageChaos(t *testing.T) {
	requireDocker(t)
	requirePortalImage(t)

	ctx := context.Background()

	// ── Step 0: Infrastructure — startup order is critical ───────────────────
	// MinIO must be up before Toxiproxy (we need mn.ContainerEndpoint to
	// configure the proxy upstream). Toxiproxy must be up before the portal
	// cluster (the cluster's JAMSESH_OBJECT_STORAGE_ENDPOINT_URL must point
	// at tp.ContainerIP:stosChaosProxyPort, not directly at MinIO).
	mn := minio.Start(ctx, t, minio.Options{})
	tp := toxiproxy.Start(ctx, t)
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// Create the Toxiproxy proxy: tp-container port 9101 → MinIO container:9000.
	// stripScheme removes "http://" to get "ip:9000" as required by Toxiproxy.
	minioUpstream := stosStripScheme(mn.ContainerEndpoint)
	tp.CreateProxy(ctx, t, stosChaosProxyName, stosChaosProxyListen, minioUpstream)

	// Configure both portal pods to route their S3 operations through Toxiproxy.
	// The test process itself uses mn.Endpoint (host-mapped, bypasses Toxiproxy)
	// for direct bucket inspection — this is intentional: the test must be able
	// to verify bucket state even when the portal→MinIO path is degraded.
	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false, // pods addressed directly; router not needed for this test
		PortalExtraEnv: map[string]string{
			// Route all portal S3 traffic through Toxiproxy.
			"JAMSESH_OBJECT_STORAGE_ENDPOINT_URL": fmt.Sprintf("http://%s:%d",
				tp.ContainerIP, stosChaosProxyPort),
			// Short heartbeat so lease state settles quickly.
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
			// SMTP for magic-link delivery.
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})

	// ── Step 1: Auth + org + session ─────────────────────────────────────────
	aliceEmail := randEmail(t, "handoff-sto-chaos")
	pair := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh, aliceEmail)
	t.Logf("handoff-sto-chaos: authenticated as %s", aliceEmail)

	userID := stosChaosGetMe(ctx, t, c.Pods[0].URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, c.Pods[0], pair.AccessToken, "Handoff ObjStorage Chaos Org")
	sessionID := stosChaosCreateSession(ctx, t, c.Pods[0].URL, pair.AccessToken, orgID, "handoff-sto-chaos-session")
	t.Logf("handoff-sto-chaos: created session %s", sessionID)

	// ── Step 2: Push 5 commits via pod 0 (no toxic — baseline RPO=0) ─────────
	// At this point Toxiproxy is running but no toxic is injected, so S3
	// writes succeed at normal speed. All 5 SHAs are acked by the push call.
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo0 := gitclient.Clone(ctx, t, c.Pods[0].URL, orgID, sessionID, userID, pair.AccessToken)

	ackedSHAs := make([]string, 5)
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("sto-chaos-commit-%02d.md", i+1)
		message := fmt.Sprintf("handoff-sto-chaos: pre-chaos commit %d of 5", i+1)
		sha := repo0.Commit(ctx, t, filename, fmt.Sprintf("content %d", i+1), message)
		ackedSHAs[i] = sha
	}
	repo0.Push(ctx, t, ref)
	t.Logf("handoff-sto-chaos: pushed 5 commits via pod 0; tip SHA = %s", ackedSHAs[4])

	// Confirm pod 0 holds the lease after the push (lazy acquisition on push).
	holder := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	if holder != 0 {
		// This is a soft requirement: we pushed directly to pod 0, so pod 0
		// should hold the lease. If the consistent-hash ring picks differently,
		// log and continue — the test's structural invariant (drain the holder
		// and wait for pod 1 to hydrate) still holds.
		t.Logf("handoff-sto-chaos: WARNING: expected pod 0 to hold lease after direct push; got pod %d — continuing", holder)
	}
	t.Logf("handoff-sto-chaos: pre-chaos lease holder = pod %d", holder)

	// Record pre-chaos draft tip on pod 0 for the downstream state assertion.
	draftTipBefore := stosChaosRefTip(ctx, t, c.Pods[0].URL, pair.AccessToken, orgID, sessionID, ref)
	if draftTipBefore == "" {
		t.Fatalf("handoff-sto-chaos: pod 0 returned empty SHA for ref %q before chaos — prerequisite failure", ref)
	}
	t.Logf("handoff-sto-chaos: pre-chaos draft tip on pod 0 = %s", draftTipBefore)

	// ── Step 3: Inject 4 000ms Toxiproxy latency ─────────────────────────────
	// The latency affects every S3 operation on both pods: pod 0's shutdown
	// flush and pod 1's hydration fetch. This simulates a degraded object-
	// storage path during the handoff window.
	tp.AddLatency(ctx, t, stosChaosProxyName, stosChaosLatencyToxicName, stosChaosLatencyMs)
	toxicActive := true
	t.Cleanup(func() {
		if toxicActive {
			tp.RemoveToxic(context.Background(), t, stosChaosProxyName, stosChaosLatencyToxicName)
		}
	})
	t.Logf("handoff-sto-chaos: injected %dms Toxiproxy latency on %s", stosChaosLatencyMs, stosChaosProxyName)

	// ── Step 4: Graceful-drain pod 0 ─────────────────────────────────────────
	// SIGTERM pod 0 and wait for clean exit. The drain may be slow because the
	// shutdown sequence flushes remaining sync writes through the toxic (4s per
	// S3 request). The drain timeout is 45s to accommodate this.
	//
	// Note: GracefulDrain uses SIGTERM, which triggers a clean shutdown. The
	// router CAN handle SIGTERM exits because the pod exits cleanly, whereas
	// SIGKILL requires direct pod addressing (bug-router-static-discoverer).
	// This test uses no router (Router: false), so both pods are addressed
	// directly throughout.
	c.GracefulDrain(ctx, t, 0, 45*time.Second)
	t.Logf("handoff-sto-chaos: drained pod 0 under %dms latency", stosChaosLatencyMs)

	// ── Step 5: Wait for pod 1 to hydrate from MinIO ─────────────────────────
	// WaitForHydration polls git ls-remote against pod 1 directly. Hydration
	// is slow because each S3 GetObject call incurs 4 000ms latency. The 45s
	// SLO is ~4-9× the expected hydration time for O(10) pack objects at 4s
	// latency with 8 parallel workers.
	//
	// If hydration HANGS indefinitely (never completes within 45s), this
	// indicates a missing timeout or deadlock in the hydration worker. Per
	// test-integrity rules: do NOT raise the SLO to infinity. Park the bug
	// via /agile-workflow:park if the 45s SLO is reliably insufficient.
	c.WaitForHydration(ctx, t, orgID, sessionID, pair.AccessToken, 1, 45*time.Second)
	t.Logf("handoff-sto-chaos: pod 1 hydrated from MinIO within 45s SLO ✓")

	// ── Step 6: Remove the toxic ──────────────────────────────────────────────
	// The chaos window closes. Subsequent S3 operations (including the 6th
	// push below) run at normal speed.
	tp.RemoveToxic(ctx, t, stosChaosProxyName, stosChaosLatencyToxicName)
	toxicActive = false
	t.Logf("handoff-sto-chaos: Toxiproxy latency removed — chaos window closed")

	// ── Step 7: Push commit 6 via pod 1 ──────────────────────────────────────
	// Pod 1 must be writable after hydration. A successful push proves the
	// session was correctly handed off with full write capability.
	repo1 := gitclient.Clone(ctx, t, c.Pods[1].URL, orgID, sessionID, userID, pair.AccessToken)
	repo1.Commit(ctx, t, "sto-chaos-commit-06.md", "post-chaos content", "handoff-sto-chaos: commit 6 post-chaos")
	repo1.Push(ctx, t, ref)
	t.Logf("handoff-sto-chaos: pushed commit 6 via pod 1 post-chaos ✓")

	// ── Step 8: Draft-tip state assertion — no acked commits lost ────────────
	// The non-tautological assertion: all 5 pre-chaos acked commit SHAs must
	// be reachable ancestors of pod 1's current ref tip. A missing SHA here
	// means a commit that was ACK'd (push returned success) before the chaos
	// window was not durably stored in MinIO — an RPO=0 violation.
	//
	// Per test-integrity rules: do NOT weaken this assertion. If it fails
	// reproducibly, park via /agile-workflow:park.
	survivorTip := stosChaosRefTip(ctx, t, c.Pods[1].URL, pair.AccessToken, orgID, sessionID, ref)
	if survivorTip == "" {
		t.Fatalf("handoff-sto-chaos: DATA LOSS — pod 1 has no SHA for ref %q after chaos+hydration; "+
			"all 5 pre-chaos acked commits are absent. Pre-chaos tip was %s. "+
			"This is a Critical durability bug — park via /agile-workflow:park.",
			ref, draftTipBefore)
	}

	stosChaosRequireAncestor(ctx, t, c.Pods[1].URL, pair.AccessToken,
		orgID, sessionID, userID, ref, draftTipBefore, ackedSHAs)

	t.Logf("handoff-sto-chaos: pod 1 tip = %s — all 5 pre-chaos acked SHAs reachable ✓", survivorTip)

	// ── Step 9: MinIO bucket has objects for the session ─────────────────────
	objectPrefix := "sessions/" + sessionID + "/"
	objects, err := mn.ListObjects(ctx, objectPrefix)
	if err != nil {
		t.Fatalf("handoff-sto-chaos: MinIO ListObjects(%q): %v", objectPrefix, err)
	}
	if len(objects) == 0 {
		t.Fatalf("handoff-sto-chaos: MinIO bucket is empty for prefix %q after chaos+handoff — "+
			"RPO=0 invariant violated; session objects must persist in object storage",
			objectPrefix)
	}
	t.Logf("handoff-sto-chaos: MinIO has %d object(s) under %s — bucket intact ✓", len(objects), objectPrefix)

	t.Logf("handoff-sto-chaos: PASS — pod 0 drained under %dms object-storage latency; "+
		"pod 1 hydrated within 45s SLO; zero data loss confirmed ✓", stosChaosLatencyMs)
}

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

// stosSessionRef captures the ID from POST /api/orgs/{id}/sessions.
type stosSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stosChaosCreateSession creates a session at baseURL and returns its ID.
func stosChaosCreateSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "handoff object-storage chaos e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("stosChaosCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("stosChaosCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stosChaosCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("stosChaosCreateSession: want 201; got %d; body: %s", resp.StatusCode, respBody)
	}
	var s stosSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("stosChaosCreateSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("stosChaosCreateSession: empty session ID; body: %s", respBody)
	}
	return s.ID
}

// stosChaosGetMe calls GET /api/me and returns the user ID.
func stosChaosGetMe(ctx context.Context, t *testing.T, podURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("stosChaosGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stosChaosGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stosChaosGetMe: want 200; got %d; body: %s", resp.StatusCode, body)
	}
	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("stosChaosGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("stosChaosGetMe: empty user ID; body: %s", body)
	}
	return me.ID
}

// stosChaosRefTip queries the pod's refs endpoint and returns the SHA for the
// given ref. Returns "" if the ref is absent.
func stosChaosRefTip(
	ctx context.Context, t *testing.T,
	podURL, accessToken, orgID, sessionID, ref string,
) string {
	t.Helper()
	type refEntry struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	}
	type refListResp struct {
		Refs []refEntry `json:"refs"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/refs", podURL, orgID, sessionID),
		nil)
	if err != nil {
		t.Fatalf("stosChaosRefTip: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stosChaosRefTip: GET refs: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stosChaosRefTip: want 200; got %d; body: %s", resp.StatusCode, body)
	}
	var rl refListResp
	if err := json.Unmarshal(body, &rl); err != nil {
		t.Fatalf("stosChaosRefTip: decode: %v; body: %s", err, body)
	}
	for _, r := range rl.Refs {
		if r.Ref == ref {
			return r.Sha
		}
	}
	return ""
}

// stosChaosRequireAncestor clones the session from podURL, fetches the remote
// ref, and verifies that draftTipBefore and each of ackedSHAs are reachable
// ancestors of the current ref tip via `git merge-base --is-ancestor`.
//
// A failure means a pre-chaos acked commit is not in the ref's ancestry on
// pod 1 after hydration — Critical RPO=0 violation. Do NOT weaken this check.
func stosChaosRequireAncestor(
	ctx context.Context, t *testing.T,
	podURL, accessToken, orgID, sessionID, userID, ref, draftTipBefore string,
	ackedSHAs []string,
) {
	t.Helper()

	repo := gitclient.Clone(ctx, t, podURL, orgID, sessionID, userID, accessToken)
	repo.Fetch(ctx, t)

	currentTip := repo.RevParse(ctx, t, ref)
	if currentTip == "" {
		t.Fatalf("stosChaosRequireAncestor: RevParse returned empty SHA for ref %q on pod 1", ref)
	}
	t.Logf("stosChaosRequireAncestor: pod 1 ref %q tip = %s", ref, currentTip)

	// The pre-chaos draft tip must be an ancestor of the current tip.
	if err := gitMergeBaseIsAncestor(ctx, repo.Dir, draftTipBefore, currentTip); err != nil {
		t.Fatalf("handoff-sto-chaos: DATA LOSS — pre-chaos draft tip %s is NOT an ancestor of "+
			"pod 1 tip %s after object-storage chaos+hydration; "+
			"acked commits were lost. Critical RPO=0 violation — "+
			"park via /agile-workflow:park: %v",
			draftTipBefore, currentTip, err)
	}
	t.Logf("stosChaosRequireAncestor: pre-chaos tip %s is ancestor of pod 1 tip %s ✓", draftTipBefore, currentTip)

	// Belt-and-suspenders: verify each individually acked SHA is also reachable.
	for i, sha := range ackedSHAs {
		if err := gitMergeBaseIsAncestor(ctx, repo.Dir, sha, currentTip); err != nil {
			t.Fatalf("handoff-sto-chaos: DATA LOSS — acked SHA[%d] %s is NOT an ancestor of "+
				"pod 1 tip %s; this commit was ACK'd pre-chaos but lost during handoff. "+
				"Critical RPO=0 violation — park via /agile-workflow:park: %v",
				i, sha, currentTip, err)
		}
		t.Logf("stosChaosRequireAncestor: acked SHA[%d] %s is ancestor of pod 1 tip %s ✓", i, sha, currentTip)
	}
}

// stosStripScheme removes a leading "http://" or "https://" scheme from addr,
// returning the bare "host:port" form required by Toxiproxy's upstream field.
func stosStripScheme(addr string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(addr, prefix) {
			return addr[len(prefix):]
		}
	}
	return addr
}
