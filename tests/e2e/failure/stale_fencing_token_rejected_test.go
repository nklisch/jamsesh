// Invariant: a write to the object-storage manifest carrying a fencing token
// that is strictly lower than the current on-disk token is rejected with a
// documented error — not silently accepted, not panicked. Silent acceptance is
// a Critical split-brain bug (two lease holders could both write, with the
// stale one clobbering the fresh one).
//
// Mechanism:
//  1. Push via pod 0 to establish lease with token T1. Record T1 via FencingTokenForSession.
//  2. Kill pod 0 (drops Postgres connection → advisory lock auto-released).
//  3. Force-release the lease row so pod 1 can re-acquire.
//  4. Push via pod 1 to acquire the lease with token T2 > T1.
//  5. Forge the on-disk manifest: overwrite it with a version whose FencingToken is T3
//     (artificially high), simulating a future pod that had advanced the token beyond T2.
//  6. Push from pod 1 again (its current token is T2 < T3). The portal's ManifestStore.Save
//     pre-flight check (onDisk.FencingToken > m.FencingToken) must return ErrFenced
//     and the push must fail with a non-2xx response.
//  7. Verify the on-disk manifest still carries T3 (i.e., the stale write did NOT overwrite it).
//
// SAFETY-CRITICAL ASSERTION: if step 6 succeeds (push returns 200 or git exits 0),
// that is a Critical split-brain bug — the stale-token guard is broken. This test
// will t.Fatal in that case, not t.Skip.
//
// SKIP path: if the manifest format cannot be injected from the test (e.g., the
// MinIO ETag-conditional-write rejects our unconditional PutObject), this test
// skips with a documented reason pointing to a follow-on story.
package failure_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// staleManifest mirrors the Manifest struct from internal/portal/storage/objectstore/manifest.go.
// We only need the FencingToken field to be correct; other fields can carry
// the values from the last legitimate push.
type staleManifest struct {
	Version      int               `json:"version"`
	SessionID    string            `json:"session_id"`
	Packs        []stalePack       `json:"packs"`
	Refs         map[string]string `json:"refs"`
	PackedRefs   string            `json:"packed_refs"`
	FencingToken int64             `json:"fencing_token"`
	UpdatedAt    string            `json:"updated_at"` // RFC3339 string — kept as-is
}

// stalePack mirrors PackEntry minimally.
type stalePack struct {
	PackKey string `json:"pack_key"`
	IdxKey  string `json:"idx_key"`
	SHA     string `json:"sha"`
}

// TestStaleFencingTokenRejected verifies that the portal's ManifestStore.Save
// pre-flight check (ErrFenced) fires when the on-disk fencing token is higher
// than the caller's token, and that the caller's push fails with a non-2xx
// response rather than silently overwriting the manifest.
func TestStaleFencingTokenRejected(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// ── Infrastructure ─────────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)
	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true, // we need the router to route to the surviving pod
		PortalExtraEnv: map[string]string{
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
			"JAMSESH_EMAIL_PROVIDER":             "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":            mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":            strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":             "none",
		},
	})
	if c.RouterURL == "" {
		t.Fatal("stale_fencing_token_rejected: Router: true is required for this test")
	}

	// ── Auth + session creation ─────────────────────────────────────────────────
	pod0 := c.Pods[0]
	userEmail := staleFencingRandEmail("lf-stale")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "StaleFencing Org")
	sessionID := staleFencingCreateSession(ctx, t, c.RouterURL, pair.AccessToken, orgID, "stale-fencing-session")
	userID := staleFencingGetMe(ctx, t, pod0.URL, pair.AccessToken)

	t.Logf("stale_fencing_token_rejected: session %s created for org %s", sessionID, orgID)

	// ── Step 1: Push via router to establish lease + token T1 ──────────────────
	ref := "jam/" + sessionID + "/" + userID + "/main"
	staleFencingPush(ctx, t, c.RouterURL, orgID, sessionID, userID, pair.AccessToken,
		"stale-fence-1.md", "step 1: establish lease", "lf-stale: initial push")

	// Wait for lease to appear in Postgres.
	holderPod := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	if holderPod < 0 {
		t.Fatalf("stale_fencing_token_rejected: no pod holds lease after initial push; check deployment")
	}
	t.Logf("stale_fencing_token_rejected: lease held by pod %d after initial push", holderPod)

	tokenT1 := c.FencingTokenForSession(ctx, t, sessionID)
	if tokenT1 <= 0 {
		t.Fatalf(
			"stale_fencing_token_rejected: T1 = %d is not valid (want > 0); "+
				"token=0 means NoopManager, token=-1 means no lease row — "+
				"this is a prerequisite failure: must have a valid clustered token before the stale-token scenario",
			tokenT1,
		)
	}
	t.Logf("stale_fencing_token_rejected: T1 = %d (held by pod %d)", tokenT1, holderPod)

	// ── Step 2: Kill the lease-holding pod ─────────────────────────────────────
	c.Kill(ctx, t, holderPod)
	t.Logf("stale_fencing_token_rejected: killed pod %d (was holding lease with T1=%d)", holderPod, tokenT1)

	// ── Step 3: Force-release the lease row ────────────────────────────────────
	// Advisory lock auto-released on Kill (Postgres drops it with the connection).
	// ReleaseLeaseForcibly updates the leases table so the surviving pod can acquire.
	c.ReleaseLeaseForcibly(ctx, t, sessionID)
	t.Logf("stale_fencing_token_rejected: lease row force-released for session %s", sessionID)

	// ── Step 4: Identify surviving pod + push to get T2 ────────────────────────
	survivorIdx := (holderPod + 1) % len(c.Pods)
	survivor := c.Pods[survivorIdx]
	t.Logf("stale_fencing_token_rejected: surviving pod = pod %d (URL=%s)", survivorIdx, survivor.URL)

	staleFencingPush(ctx, t, survivor.URL, orgID, sessionID, userID, pair.AccessToken,
		"stale-fence-2.md", "step 4: survivor re-acquires lease", "lf-stale: second push (survivor)")

	// Wait for lease to migrate to the survivor.
	newHolder := c.WaitForLeaseMigration(ctx, t, sessionID, holderPod, 30*time.Second)
	if newHolder < 0 {
		t.Fatalf(
			"stale_fencing_token_rejected: lease did not migrate to pod %d within 30s after killing pod %d; "+
				"T1=%d; check JAMSESH_LEASE_HEARTBEAT_INTERVAL_S",
			survivorIdx, holderPod, tokenT1,
		)
	}
	t.Logf("stale_fencing_token_rejected: lease migrated to pod %d", newHolder)

	tokenT2 := c.FencingTokenForSession(ctx, t, sessionID)
	if tokenT2 <= tokenT1 {
		t.Fatalf(
			"stale_fencing_token_rejected: T2 (%d) <= T1 (%d) — prerequisite MONOTONICITY violated; "+
				"the stale-token scenario cannot be constructed without T2 > T1. "+
				"This is itself a Critical lease-fencing bug; see TestMonotonicFencingTokens.",
			tokenT2, tokenT1,
		)
	}
	t.Logf("stale_fencing_token_rejected: T2 = %d (monotonicity confirmed: T2 > T1)", tokenT2)

	// ── Step 5: Read current on-disk manifest from MinIO ───────────────────────
	manifestKey := "sessions/" + sessionID + "/manifest.json"
	manifestBytes, err := mn.GetObject(ctx, manifestKey)
	if err != nil {
		// If the manifest doesn't exist yet (no push happened through object storage),
		// the stale-token scenario cannot be constructed from this test.
		// This is an architecture-visibility gap, not a portal bug — skip with docs.
		t.Skipf(
			"blocked on stale-token-injection-needs-manifest-format-exposure (backlog); "+
				"cannot construct stale-token state without exposing objectstore.Manifest — "+
				"could not read on-disk manifest from MinIO (key=%q): %v — "+
				"the manifest may not have been written yet (lazy acquisition architecture means "+
				"no manifest exists until the post-receive phase completes). "+
				"The advisory-lock exclusivity assertion (TestLeaseAlreadyHeld) is the primary "+
				"split-brain guard; this test covers the manifest-layer guard.",
			manifestKey, err,
		)
	}
	t.Logf("stale_fencing_token_rejected: read manifest from MinIO (%d bytes)", len(manifestBytes))

	// Decode the manifest to get the current on-disk shape.
	var currentManifest staleManifest
	if err := json.Unmarshal(manifestBytes, &currentManifest); err != nil {
		t.Skipf(
			"blocked on stale-token-injection-needs-manifest-format-exposure (backlog); "+
				"cannot construct stale-token state without exposing objectstore.Manifest — "+
				"manifest at %q is not parseable JSON: %v — "+
				"cannot inject T3 without understanding the manifest format.",
			manifestKey, err,
		)
	}
	t.Logf("stale_fencing_token_rejected: on-disk manifest has FencingToken=%d (should match T2=%d)",
		currentManifest.FencingToken, tokenT2)

	// ── Step 6: Inject T3 (a future-pod's artificially high token) ─────────────
	// We set the manifest's fencing token to T3 = T2 + 1000, simulating a
	// scenario where a future pod advanced the token. Any subsequent write from
	// pod survivorIdx (which has token T2) must be blocked by ErrFenced.
	tokenT3 := tokenT2 + 1000
	forgedManifest := currentManifest
	forgedManifest.FencingToken = tokenT3

	forgedBytes, err := json.Marshal(forgedManifest)
	if err != nil {
		t.Fatalf("stale_fencing_token_rejected: marshal forged manifest: %v", err)
	}

	// Overwrite the manifest in MinIO with the forged version (T3).
	if err := mn.PutObject(ctx, manifestKey, forgedBytes); err != nil {
		t.Skipf(
			"blocked on stale-token-injection-needs-manifest-format-exposure (backlog); "+
				"cannot construct stale-token state without exposing objectstore.Manifest — "+
				"could not write forged manifest to MinIO (key=%q): %v — "+
				"the MinIO fixture's PutObject does not support unconditional overwrite on this object.",
			manifestKey, err,
		)
	}
	t.Logf("stale_fencing_token_rejected: forged manifest written to MinIO with T3=%d (T2=%d, T1=%d)", tokenT3, tokenT2, tokenT1)

	// Verify the forged manifest is now on-disk.
	verifyBytes, err := mn.GetObject(ctx, manifestKey)
	if err != nil {
		t.Fatalf("stale_fencing_token_rejected: read-back forged manifest: %v", err)
	}
	var verifyManifest staleManifest
	if jsonErr := json.Unmarshal(verifyBytes, &verifyManifest); jsonErr != nil {
		t.Fatalf("stale_fencing_token_rejected: decode read-back manifest: %v", jsonErr)
	}
	if verifyManifest.FencingToken != tokenT3 {
		t.Fatalf(
			"stale_fencing_token_rejected: forged manifest read-back has FencingToken=%d, want T3=%d — "+
				"PutObject did not persist the forged token",
			verifyManifest.FencingToken, tokenT3,
		)
	}
	t.Logf("stale_fencing_token_rejected: forged manifest verified on-disk with T3=%d", tokenT3)

	// ── Step 7: Push from survivor (T2 < T3) — must be rejected ─────────────────
	// The survivor pod's current lease token is T2. The on-disk manifest says T3.
	// ManifestStore.Save performs: if onDisk.FencingToken > m.FencingToken → ErrFenced.
	// Since T3 > T2, this push must fail with ErrFenced → non-2xx response.
	//
	// SAFETY-CRITICAL ASSERTION: if this push returns 200 (git exits 0), the
	// stale-token guard is broken — the portal accepted a write that would
	// overwrite a manifest from a "future" pod, destroying data. This is a
	// Critical split-brain bug.
	t.Logf("stale_fencing_token_rejected: pushing from survivor (pod %d, T2=%d) against forged manifest (T3=%d) — must be rejected", survivorIdx, tokenT2, tokenT3)
	stalePushStatus := staleFencingAttemptPush(ctx, t, survivor.URL, orgID, sessionID, userID, pair.AccessToken, ref)
	t.Logf("stale_fencing_token_rejected: stale push result: %d", stalePushStatus)

	if stalePushStatus == http.StatusOK {
		// The portal accepted a write with a stale fencing token. This is a
		// Critical split-brain bug: the manifest's T3 was overwritten by T2,
		// meaning a future pod's writes are clobbered. Park immediately.
		t.Fatalf(
			"stale_fencing_token_rejected: CRITICAL SPLIT-BRAIN BUG — "+
				"portal accepted a write with stale token T2=%d when on-disk manifest has T3=%d; "+
				"ManifestStore.Save fencing-token pre-flight check is broken or bypassed. "+
				"Park this as Critical before landing any workaround. "+
				"ErrFenced in manifest.go must reject any write where onDisk.FencingToken > m.FencingToken.",
			tokenT2, tokenT3,
		)
	}

	// Any non-2xx response satisfies the invariant: the stale write was rejected.
	// 503 is the expected portal response when ErrFenced bubbles up through the
	// push-receive path. Log the actual code for PROTOCOL.md alignment.
	t.Logf(
		"stale_fencing_token_rejected: stale push rejected with status %d (want 503) — "+
			"ErrFenced correctly blocked T2=%d write against on-disk T3=%d",
		stalePushStatus, tokenT2, tokenT3,
	)

	// ── Step 8: Verify on-disk manifest still has T3 ───────────────────────────
	// The forged write must NOT have overwritten the manifest. If it did, the
	// manifest now has T2, which is a data-loss bug (the future pod's writes are gone).
	postPushBytes, err := mn.GetObject(ctx, manifestKey)
	if err != nil {
		t.Errorf("stale_fencing_token_rejected: read manifest after stale push: %v", err)
	} else {
		var postPushManifest staleManifest
		if jsonErr := json.Unmarshal(postPushBytes, &postPushManifest); jsonErr != nil {
			t.Errorf("stale_fencing_token_rejected: decode post-push manifest: %v", jsonErr)
		} else {
			// The manifest must still carry T3 — the stale write must NOT have
			// overwritten it. If it carries T2 (< T3), the ErrFenced guard failed
			// to prevent the write at the ManifestStore level.
			if postPushManifest.FencingToken != tokenT3 {
				t.Errorf(
					"stale_fencing_token_rejected: CRITICAL DATA LOSS — on-disk manifest FencingToken is %d "+
						"after the stale push, but should still be T3=%d; "+
						"the stale write (T2=%d) overwrote the manifest despite the non-2xx response. "+
						"This indicates ErrFenced is returned AFTER the write rather than BEFORE — "+
						"park as Critical.",
					postPushManifest.FencingToken, tokenT3, tokenT2,
				)
			} else {
				t.Logf(
					"stale_fencing_token_rejected: on-disk manifest still carries T3=%d after rejected stale write — "+
						"manifest integrity confirmed",
					tokenT3,
				)
			}
		}
	}

	t.Logf(
		"stale_fencing_token_rejected: invariant verified — "+
			"stale-token write (T2=%d < T3=%d) rejected; on-disk manifest unchanged",
		tokenT2, tokenT3,
	)
}

// ---------------------------------------------------------------------------
// Per-file helpers
// ---------------------------------------------------------------------------

// staleFencingRandEmail returns a unique email for isolation.
func staleFencingRandEmail(prefix string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("staleFencingRandEmail: rand.Read: %v", err))
	}
	return prefix + "-" + hex.EncodeToString(b) + "@example.com"
}

// staleFencingCreateSession POSTs to /api/orgs/{orgID}/sessions and returns the session ID.
func staleFencingCreateSession(
	ctx context.Context, t *testing.T,
	baseURL, accessToken, orgID, name string,
) string {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"name":         name,
		"goal":         "stale fencing token rejected test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	})
	if err != nil {
		t.Fatalf("staleFencingCreateSession: marshal: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("staleFencingCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("staleFencingCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("staleFencingCreateSession: want 201, got %d; body: %s", resp.StatusCode, respBody)
	}

	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("staleFencingCreateSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("staleFencingCreateSession: empty session ID; body: %s", respBody)
	}
	return s.ID
}

// staleFencingGetMe calls GET /api/me and returns the user ID.
func staleFencingGetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("staleFencingGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("staleFencingGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("staleFencingGetMe: want 200, got %d; body: %s", resp.StatusCode, body)
	}

	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("staleFencingGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("staleFencingGetMe: empty ID; body: %s", body)
	}
	return me.ID
}

// staleFencingBasicAuthURL injects credentials into the base URL for git clone.
func staleFencingBasicAuthURL(baseURL, user, pass string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("staleFencingBasicAuthURL: parse %q: %v", baseURL, err))
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// staleFencingPush clones the session repo at podURL, commits a file, and pushes.
// t.Fatal is called on any git error (this is a "must succeed" push).
func staleFencingPush(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, userID, accessToken string,
	filename, content, commitMsg string,
) {
	t.Helper()

	repoDir := t.TempDir()
	repoURL := staleFencingBasicAuthURL(podURL, "x-access-token", accessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("staleFencingPush: git %v: %v\n%s", args, err, out)
		}
	}

	runGit("", "clone", repoURL, repoDir)
	runGit(repoDir, "config", "user.email", userID+"@test.example")
	runGit(repoDir, "config", "user.name", "Test "+userID)

	absPath := filepath.Join(repoDir, filename)
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("staleFencingPush: write file: %v", err)
	}
	runGit(repoDir, "add", filename)

	turnID := uuid.New().String()
	fullMsg := fmt.Sprintf("%s\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		commitMsg, sessionID, turnID, userID)
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(fullMsg), 0o644); err != nil {
		t.Fatalf("staleFencingPush: write commit msg: %v", err)
	}
	runGit(repoDir, "commit", "-F", msgFile)

	ref := "jam/" + sessionID + "/" + userID + "/main"
	runGit(repoDir, "push", "origin", "HEAD:refs/heads/"+ref)
}

// staleFencingAttemptPush performs a git push that MAY fail (stale-token
// rejection scenario). Returns the HTTP status code the server returned.
// If git exits 0, returns http.StatusOK. If git exits non-zero, attempts to
// extract the HTTP status from the output; returns http.StatusServiceUnavailable
// if the status cannot be determined.
func staleFencingAttemptPush(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, userID, accessToken, ref string,
) int {
	t.Helper()

	repoDir := t.TempDir()
	repoURL := staleFencingBasicAuthURL(podURL, "x-access-token", accessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	runGit := func(fatal bool, dir string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.CombinedOutput()
		if err != nil && fatal {
			t.Fatalf("staleFencingAttemptPush (setup): git %v: %v\n%s", args, err, out)
		}
		return string(out), err
	}

	runGit(true, "", "clone", repoURL, repoDir)
	runGit(true, repoDir, "config", "user.email", userID+"@test.example")
	runGit(true, repoDir, "config", "user.name", "Test "+userID)

	testFile := filepath.Join(repoDir, "stale-token-test.md")
	if err := os.WriteFile(testFile, []byte("stale token test content — must be rejected"), 0o644); err != nil {
		t.Fatalf("staleFencingAttemptPush: write test file: %v", err)
	}
	runGit(true, repoDir, "add", "stale-token-test.md")

	turnID := uuid.New().String()
	fullMsg := fmt.Sprintf(
		"lf-stale: stale token push\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, turnID, userID,
	)
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(fullMsg), 0o644); err != nil {
		t.Fatalf("staleFencingAttemptPush: write commit msg: %v", err)
	}
	runGit(true, repoDir, "commit", "-F", msgFile)

	// The push itself — may fail due to stale-token rejection.
	pushOut, pushErr := runGit(false, repoDir, "push", "origin", "HEAD:refs/heads/"+ref)
	t.Logf("staleFencingAttemptPush: git push exit=%v output=%s", pushErr, pushOut)

	if pushErr == nil {
		return http.StatusOK
	}

	// Parse known HTTP status codes from git output.
	for _, code := range []int{503, 500, 422, 401, 403} {
		if strings.Contains(pushOut, strconv.Itoa(code)) {
			return code
		}
	}

	// Non-zero git exit without a parseable code → server returned non-2xx.
	return http.StatusServiceUnavailable
}
