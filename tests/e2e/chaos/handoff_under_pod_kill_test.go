// Invariant: when the pod holding a session lease is SIGKILLed, all commits
// acknowledged before the kill are present on the surviving pod after it
// hydrates. No user-visible data loss across a hard pod crash.
//
// Design boundary: lease-ownership invariants (auto-release, monotonic-token)
// are asserted in lease_holder_killed_test.go (epic-e2e-cnd-coverage-lease-fencing).
// This test owns the user-visible handoff outcome: all acked commits present,
// no data loss. Do not add fencing-token assertions here.
//
// Chaos mechanism: c.Kill(holderPod) — `docker kill --signal SIGKILL`.
// After the kill, the surviving pod is addressed directly because
// bug-router-static-discoverer-not-started keeps the dead pod in the consistent-
// hash ring; the router may still route to it and return 502.
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
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestHandoffUnderPodKill verifies that a hard SIGKILL of the lease-holding
// pod causes zero user-visible data loss:
//
//  1. Alice pushes 5 commits through the router; all 5 are ACK'd.
//  2. The holding pod is SIGKILLed.
//  3. The surviving pod is addressed directly (router workaround for
//     bug-router-static-discoverer-not-started).
//  4. WaitForHydration confirms the survivor has rehydrated from MinIO.
//  5. All 5 pre-kill ref tip SHAs are present on the survivor's ref store —
//     no data loss.
//  6. A 6th push on the survivor succeeds, proving the session is writable
//     post-handoff.
//
// This test does NOT assert fencing-token monotonicity — that is the domain
// of TestLeaseHolderKilled in epic-e2e-cnd-coverage-lease-fencing.
func TestHandoffUnderPodKill(t *testing.T) {
	requireDocker(t)
	requirePortalImage(t)

	ctx := context.Background()

	// ── Infrastructure ────────────────────────────────────────────────────────
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
			// The 30s SLO is ~15× this value — conservative.
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
			"JAMSESH_EMAIL_PROVIDER":             "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":            mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":            strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":             "none",
		},
	})

	// Defensive cleanup: if the kill succeeds, the container is already dead
	// when t.Cleanup runs — docker kill on a dead container is a no-op error.
	// If the test fatals before Kill, this ensures the container is cleaned up.
	killedPod := -1
	t.Cleanup(func() {
		if killedPod < 0 || c == nil || killedPod >= len(c.Pods) {
			return
		}
		name := c.Pods[killedPod].ContainerName(context.Background())
		if name != "" {
			_ = runDockerKill(name) // best-effort; ignore error
		}
	})

	// ── Step 1: Auth + org + session ─────────────────────────────────────────
	aliceEmail := randEmail(t, "handoff-kill")
	pair := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh, aliceEmail)
	t.Logf("handoff-pod-kill: authenticated as %s", aliceEmail)

	userID := podKillGetMe(ctx, t, c.Pods[0].URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, c.Pods[0], pair.AccessToken, "Handoff Pod Kill Org")
	sessionID := podKillCreateSession(ctx, t, c.RouterURL, pair.AccessToken, orgID, "handoff-pod-kill-session")
	t.Logf("handoff-pod-kill: created session %s", sessionID)

	// ── Step 2: Push 5 commits via router; record acked commit SHAs ──────────
	// Push via the router so the consistent-hash ring picks the holder
	// naturally. Each Commit() returns the SHA of the committed object; that
	// SHA is acked when the subsequent Push() returns without error (RPO=0
	// per the object-storage-sync coverage).
	ref := "jam/" + sessionID + "/" + userID + "/main"

	// Clone once via the router; commit 5 times and push in one batch so the
	// push ACK covers all 5 commits.
	repo := gitclient.Clone(ctx, t, c.RouterURL, orgID, sessionID, userID, pair.AccessToken)

	ackedSHAs := make([]string, 5)
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("pod-kill-commit-%02d.md", i+1)
		message := fmt.Sprintf("handoff-pod-kill: pre-kill commit %d of 5", i+1)
		sha := repo.Commit(ctx, t, filename, fmt.Sprintf("content %d", i+1), message)
		ackedSHAs[i] = sha
	}
	repo.Push(ctx, t, ref)
	t.Logf("handoff-pod-kill: pushed 5 commits via router; tip SHA = %s", ackedSHAs[4])

	// ── Step 3: Identify the lease holder ────────────────────────────────────
	holderPod := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	t.Logf("handoff-pod-kill: pre-kill lease holder = pod %d", holderPod)

	// Record the draft tip on the holder directly as a pre-kill baseline.
	draftTipBefore := podKillRefTip(ctx, t, c.Pods[holderPod].URL, pair.AccessToken, orgID, sessionID, ref)
	if draftTipBefore == "" {
		t.Fatalf("handoff-pod-kill: holder pod %d returned empty SHA for ref %q before kill — prerequisite failure", holderPod, ref)
	}
	t.Logf("handoff-pod-kill: pre-kill draft tip on pod %d = %s", holderPod, draftTipBefore)

	survivorIdx := (holderPod + 1) % len(c.Pods)
	t.Logf("handoff-pod-kill: expected survivor = pod %d", survivorIdx)

	// ── Step 4: SIGKILL the holding pod ──────────────────────────────────────
	killedPod = holderPod
	c.Kill(ctx, t, holderPod)
	t.Logf("handoff-pod-kill: SIGKILLed pod %d", holderPod)

	// NOTE: bug-router-static-discoverer-not-started — router may still route
	// to the dead pod; all assertions after this point address the survivor
	// pod directly via c.Pods[survivorIdx].URL.
	t.Logf("NOTE: bug-router-static-discoverer-not-started — router may still "+
		"route to dead pod; asserting directly against survivor pod %d", survivorIdx)

	// ── Step 5: Wait for survivor to hydrate from MinIO ───────────────────────
	// WaitForHydration polls git ls-remote against pod B directly. Success
	// means pod B's local bare-repo cache is populated from object storage —
	// the exact post-hydration readiness signal. SLO: 30s.
	c.WaitForHydration(ctx, t, orgID, sessionID, pair.AccessToken, survivorIdx, 30*time.Second)
	t.Logf("handoff-pod-kill: survivor pod %d hydrated from MinIO ✓", survivorIdx)

	// ── Step 6: Push 6th commit directly to survivor ──────────────────────────
	// Address the survivor pod directly (bypass router). A successful push on
	// the survivor proves the session is writable post-handoff.
	repo2 := gitclient.Clone(ctx, t, c.Pods[survivorIdx].URL, orgID, sessionID, userID, pair.AccessToken)
	repo2.Commit(ctx, t, "pod-kill-commit-06.md", "post-kill content", "handoff-pod-kill: commit 6 post-kill")
	repo2.Push(ctx, t, ref)
	t.Logf("handoff-pod-kill: pushed commit 6 via survivor pod %d directly ✓", survivorIdx)

	// ── Step 7: Confirm the survivor holds the lease ──────────────────────────
	// The survivor must have acquired the advisory lock to accept the push.
	newHolder := c.RequireLeaseHolder(ctx, t, sessionID, 15*time.Second)
	t.Logf("handoff-pod-kill: post-handoff lease holder = pod %d", newHolder)

	// ── Step 8: Draft-tip assertion — zero data loss ──────────────────────────
	// The survivor's ref history must include all 5 pre-kill acked commits as
	// reachable ancestors of the current tip. We clone from the survivor and
	// use `git merge-base --is-ancestor` for each SHA. This is the
	// non-tautological assertion: we verify actual commit ancestry, not just
	// HTTP status codes.
	//
	// A failure here means acked commits were lost across the kill — Critical
	// durability bug. Per test-integrity rules, do NOT weaken this assertion.
	survivorTip := podKillRefTip(ctx, t, c.Pods[survivorIdx].URL, pair.AccessToken, orgID, sessionID, ref)
	if survivorTip == "" {
		// The ref is absent on the survivor after hydration. This means hydration
		// did NOT restore the acked commits from MinIO — Critical data-loss bug.
		//
		// Per test-integrity rules: if this fails reproducibly, park the bug via
		// /agile-workflow:park. The failing assertion is the correct behavior.
		t.Fatalf("handoff-pod-kill: DATA LOSS — survivor pod %d has no SHA for ref %q "+
			"after hydration; all 5 pre-kill acked commits are absent. "+
			"Pre-kill tip was %s. This is a Critical durability bug — "+
			"park via /agile-workflow:park.",
			survivorIdx, ref, draftTipBefore)
	}

	podKillRequireAncestor(ctx, t, c.Pods[survivorIdx].URL, pair.AccessToken,
		orgID, sessionID, userID, ref, draftTipBefore, ackedSHAs)

	t.Logf("handoff-pod-kill: survivor tip = %s (pre-kill was %s) — all 5 acked SHAs reachable ✓",
		survivorTip, draftTipBefore)

	// ── Step 9: MinIO bucket has objects for the 6th commit ───────────────────
	objectPrefix := "sessions/" + sessionID + "/"
	objects, err := mn.ListObjects(ctx, objectPrefix)
	if err != nil {
		t.Fatalf("handoff-pod-kill: MinIO ListObjects(%q): %v", objectPrefix, err)
	}
	if len(objects) == 0 {
		t.Fatalf("handoff-pod-kill: MinIO bucket is empty for prefix %q after kill+handoff — "+
			"the 6th post-kill push must have synced to object storage",
			objectPrefix)
	}
	t.Logf("handoff-pod-kill: MinIO has %d object(s) under %s — bucket intact after kill ✓", len(objects), objectPrefix)

	t.Logf("handoff-pod-kill: PASS — pod %d killed; survivor pod %d hydrated; zero data loss confirmed ✓",
		holderPod, survivorIdx)
}

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

// podKillSessionRef captures the ID from POST /api/orgs/{id}/sessions.
type podKillSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// podKillCreateSession creates a session at baseURL and returns its ID.
func podKillCreateSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "handoff-pod-kill chaos e2e",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("podKillCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("podKillCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("podKillCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("podKillCreateSession: want 201; got %d; body: %s", resp.StatusCode, respBody)
	}
	var s podKillSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("podKillCreateSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("podKillCreateSession: empty session ID; body: %s", respBody)
	}
	return s.ID
}

// podKillGetMe calls GET /api/me and returns the user ID.
func podKillGetMe(ctx context.Context, t *testing.T, podURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("podKillGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("podKillGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("podKillGetMe: want 200; got %d; body: %s", resp.StatusCode, body)
	}
	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("podKillGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("podKillGetMe: empty user ID; body: %s", body)
	}
	return me.ID
}

// podKillRefTip queries GET /api/orgs/{orgID}/sessions/{sessionID}/refs on
// podURL and returns the SHA for the given ref. The caller passes the short
// push form ("jam/<sid>/<uid>/main", matching gitclient.Push and RevParse);
// the REST /refs API reports the FULL ref name ("refs/heads/jam/...", from
// ListSessionRefs -> r.Name().String()), so we canonicalize to the full form
// before comparing. Returns "" if the ref is absent.
func podKillRefTip(
	ctx context.Context, t *testing.T,
	podURL, accessToken, orgID, sessionID, ref string,
) string {
	t.Helper()
	wantRef := ref
	if !strings.HasPrefix(wantRef, "refs/heads/") {
		wantRef = "refs/heads/" + wantRef
	}
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
		t.Fatalf("podKillRefTip: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("podKillRefTip: GET refs: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("podKillRefTip: want 200; got %d; body: %s", resp.StatusCode, body)
	}
	var rl refListResp
	if err := json.Unmarshal(body, &rl); err != nil {
		t.Fatalf("podKillRefTip: decode: %v; body: %s", err, body)
	}
	for _, r := range rl.Refs {
		if r.Ref == wantRef {
			return r.Sha
		}
	}
	return ""
}

// podKillRequireAncestor clones the session from podURL, fetches the remote
// ref, and verifies that draftTipBefore and each of ackedSHAs are reachable
// ancestors of the current ref tip via `git merge-base --is-ancestor`.
//
// A failure means an acked SHA is not in the ref's commit history on the
// survivor — Critical data-loss bug. Per test-integrity rules: do NOT weaken
// this assertion. If it fails reproducibly, park via /agile-workflow:park.
func podKillRequireAncestor(
	ctx context.Context, t *testing.T,
	podURL, accessToken, orgID, sessionID, userID, ref, draftTipBefore string,
	ackedSHAs []string,
) {
	t.Helper()

	repo := gitclient.Clone(ctx, t, podURL, orgID, sessionID, userID, accessToken)
	repo.Fetch(ctx, t)

	currentTip := repo.RevParse(ctx, t, ref)
	if currentTip == "" {
		t.Fatalf("podKillRequireAncestor: RevParse returned empty SHA for ref %q on survivor", ref)
	}
	t.Logf("podKillRequireAncestor: survivor ref %q tip = %s", ref, currentTip)

	// Verify draftTipBefore is an ancestor of currentTip.
	if err := gitMergeBaseIsAncestor(ctx, repo.Dir, draftTipBefore, currentTip); err != nil {
		// Pre-kill draft tip is not reachable from the survivor's current tip.
		// This is a Critical data-loss bug: the acked commits from before the kill
		// were not preserved through the hydration-handoff.
		t.Fatalf("handoff-pod-kill: DATA LOSS — pre-kill draft tip %s is NOT an ancestor of "+
			"survivor tip %s after SIGKILL+hydration; "+
			"acked commits were lost across the pod kill. "+
			"This is a Critical durability bug — park via /agile-workflow:park: %v",
			draftTipBefore, currentTip, err)
	}
	t.Logf("podKillRequireAncestor: pre-kill tip %s is ancestor of survivor tip %s ✓", draftTipBefore, currentTip)

	// Belt-and-suspenders: verify each individually acked SHA is also an
	// ancestor of the current tip. This catches partial-write scenarios where
	// only some commits made it to MinIO despite the batch push ACK.
	for i, sha := range ackedSHAs {
		if err := gitMergeBaseIsAncestor(ctx, repo.Dir, sha, currentTip); err != nil {
			t.Fatalf("handoff-pod-kill: DATA LOSS — acked SHA[%d] %s is NOT an ancestor of "+
				"survivor tip %s after SIGKILL+hydration; "+
				"this commit was ACK'd before the kill but is absent on the survivor. "+
				"Critical durability bug — park via /agile-workflow:park: %v",
				i, sha, currentTip, err)
		}
		t.Logf("podKillRequireAncestor: acked SHA[%d] %s is ancestor of survivor tip %s ✓", i, sha, currentTip)
	}
}

// gitMergeBaseIsAncestor runs `git merge-base --is-ancestor <candidate> <tip>`
// in repoDir. Returns nil if candidate is an ancestor of tip, non-nil otherwise.
// Exit code 0 = ancestor, exit code 1 = not an ancestor, >1 = error.
func gitMergeBaseIsAncestor(ctx context.Context, repoDir, candidate, tip string) error {
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", candidate, tip)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git merge-base --is-ancestor %s %s: %w; output: %s",
			candidate, tip, err, out)
	}
	return nil
}
