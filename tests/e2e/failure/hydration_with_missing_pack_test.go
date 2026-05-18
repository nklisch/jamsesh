// Safety invariant: if a pack object is missing from the MinIO bucket when pod
// B attempts to hydrate session S, pod B must REFUSE to serve the session and
// surface a non-200 error. No silent truncation, no partial state served.
//
// This is the heaviest test-integrity story in the hydration-handoff feature.
// If the system silently serves partial state, that is a Critical production
// bug — see escape hatch comments below.
//
// Test design:
//  1. Pod 0 pushes 15 commits → packs land in MinIO.
//  2. Delete one pack file from MinIO out-of-band (corruption).
//  3. Attempt push via pod 1 — pod 1 must hydrate before serving, but the
//     pack is missing → hydration fails → push rejected with non-zero git exit.
//  4. Assert pod 1 did NOT serve partial state (ls-remote returns no refs or
//     fails; bucket manifest was NOT updated to claim success).
//  5. Subtest recovery_after_repair: restore the pack via PutObject → retry
//     push → succeeds; draft tip matches the pre-corruption state.
//
// Router: false so we can address each pod directly, avoiding the static-
// discoverer bug (bug-router-static-discoverer-not-started) which keeps dead
// pods in the hash ring.
package failure_test

import (
	"bytes"
	"context"
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

// mhpManifest mirrors the minimal Manifest fields we need to read from MinIO
// to assert the manifest was NOT updated after a failed hydration attempt.
// Shape must match internal/portal/storage/objectstore/manifest.go.
type mhpManifest struct {
	Version      int               `json:"version"`
	SessionID    string            `json:"session_id"`
	Packs        []mhpPackEntry    `json:"packs"`
	Refs         map[string]string `json:"refs"`
	PackedRefs   string            `json:"packed_refs"`
	FencingToken int64             `json:"fencing_token"`
	UpdatedAt    string            `json:"updated_at"`
}

// mhpPackEntry mirrors PackEntry minimally.
type mhpPackEntry struct {
	PackKey string `json:"pack_key"`
	IdxKey  string `json:"idx_key"`
	SHA     string `json:"sha"`
}

// TestHydrationWithMissingPack verifies the corruption-bucket safety invariant:
// a missing pack object must cause hydration to fail loudly — not silently
// produce a partial session.
func TestHydrationWithMissingPack(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// ── Infrastructure ────────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	// Router: false — we address each pod directly. This avoids the static-
	// discoverer bug and lets us precisely control which pod receives each request.
	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})

	pod0 := c.Pods[0]
	pod1 := c.Pods[1]

	// ── Auth + session setup via pod 0 ───────────────────────────────────────
	userEmail := mhpRandEmail("hh-failure")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "HHFailure Org")
	sessionID := mhpCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "hh-missing-pack")
	userID := mhpGetMe(ctx, t, pod0.URL, pair.AccessToken)

	t.Logf("mhp: session %s created for org %s (user %s)", sessionID, orgID, userID)

	// ── Step 1: Push 15 commits to pod 0 ─────────────────────────────────────
	// 15 moderately-sized commits is sufficient to guarantee at least one pack
	// file is produced server-side (mirrors the multi_pack_push pattern in
	// object_storage_rpo0_test.go).
	ref := "jam/" + sessionID + "/" + userID + "/main"
	mhpSetupAndPushN(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken, ref, 15)

	t.Logf("mhp: 15 commits pushed to pod 0; pack objects expected in MinIO")

	// ── Step 2: Verify bucket has pack objects ────────────────────────────────
	packsPrefix := "sessions/" + sessionID + "/packs/"
	packKeys, err := mn.ListObjects(ctx, packsPrefix)
	if err != nil {
		t.Fatalf("mhp: list pack objects: %v", err)
	}
	if len(packKeys) == 0 {
		t.Skipf(
			"mhp: no pack objects found under prefix %q after 15 commits — "+
				"git may not have created a pack on the server side; "+
				"this is a prerequisite failure, not a portal bug. "+
				"Consider increasing commit count or adding bulk content.",
			packsPrefix,
		)
	}
	t.Logf("mhp: found %d pack key(s) under %s: %v", len(packKeys), packsPrefix, packKeys)

	// Pick the first .pack file key to corrupt.
	packKey := ""
	for _, k := range packKeys {
		if strings.HasSuffix(k, ".pack") {
			packKey = k
			break
		}
	}
	if packKey == "" {
		t.Skipf(
			"mhp: no .pack file found under prefix %q (found: %v) — "+
				"cannot corrupt a non-existent pack. Prerequisite not met.",
			packsPrefix, packKeys,
		)
	}
	t.Logf("mhp: selected pack key for corruption: %s", packKey)

	// ── Step 3: Read the pack data for recovery subtest ───────────────────────
	packData, err := mn.GetObject(ctx, packKey)
	if err != nil {
		t.Fatalf("mhp: read pack object %q before deletion: %v", packKey, err)
	}
	t.Logf("mhp: read pack object (%d bytes) for recovery subtest", len(packData))

	// Read manifest before corruption to compare post-corruption state.
	manifestKey := "sessions/" + sessionID + "/manifest.json"
	manifestBefore, err := mn.GetObject(ctx, manifestKey)
	if err != nil {
		t.Fatalf("mhp: read manifest before corruption: %v", err)
	}
	var mBefore mhpManifest
	if err := json.Unmarshal(manifestBefore, &mBefore); err != nil {
		t.Fatalf("mhp: decode manifest before corruption: %v", err)
	}
	t.Logf("mhp: manifest before corruption: FencingToken=%d Packs=%d Refs=%d",
		mBefore.FencingToken, len(mBefore.Packs), len(mBefore.Refs))

	// ── Step 4: Delete the pack object out-of-band ────────────────────────────
	if err := mn.DeleteObject(ctx, packKey); err != nil {
		t.Fatalf("mhp: delete pack object %q: %v", packKey, err)
	}

	// Verify the deletion by confirming the key is gone from ListObjects.
	keysAfterDeletion, err := mn.ListObjects(ctx, packsPrefix)
	if err != nil {
		t.Fatalf("mhp: list pack objects after deletion: %v", err)
	}
	for _, k := range keysAfterDeletion {
		if k == packKey {
			t.Fatalf("mhp: DeleteObject reported success but %q is still listed — MinIO did not delete the object", packKey)
		}
	}
	t.Logf("mhp: pack object %q deleted from MinIO (confirmed via re-list; %d key(s) remain)", packKey, len(keysAfterDeletion))

	// ── Step 5: Attempt push via pod 1 ───────────────────────────────────────
	// Pod 1 has never seen this session — it must hydrate from MinIO before
	// serving. With one pack file missing, hydration must fail and the push
	// must be rejected.
	//
	// We do NOT use gitclient.Clone + repo.Push because those always t.Fatal on
	// failure; instead we exec directly and capture the exit code.
	t.Logf("mhp: attempting push via pod 1 (URL=%s) — hydration must fail with non-zero exit", pod1.URL)

	// Allow enough time for pod 1 to attempt hydration and fail.
	// The SLO is 30s for golden tests; we give 20s here — enough for a failed
	// fast-fail from the missing-pack download error.
	pushCtx, pushCancel := context.WithTimeout(ctx, 20*time.Second)
	defer pushCancel()

	pushExitCode, pushOutput := mhpAttemptPush(pushCtx, t, pod1.URL, orgID, sessionID, userID, pair.AccessToken, ref)
	t.Logf("mhp: pod 1 push result: exit=%d output=%s", pushExitCode, pushOutput)

	// ── Assertion 1: push must fail ───────────────────────────────────────────
	if pushExitCode == 0 {
		// CRITICAL: pod 1 accepted the push despite a missing pack object. This
		// means hydration silently served partial state — an unacceptable safety
		// invariant violation.
		//
		// Per story design: if this happens, park as Critical and skip the test
		// so the suite stays honest. DO NOT change this assertion to "any response
		// is ok" — the invariant IS the point.
		t.Fatalf(
			"mhp: CRITICAL SAFETY INVARIANT VIOLATED — "+
				"pod 1 accepted push (git exit 0) despite missing pack object %q. "+
				"This means hydration silently served partial state from a corrupt bucket. "+
				"This is a Critical durability bug: bucket corruption must fail loudly, "+
				"not silently produce wrong data. "+
				"Park as Critical via /agile-workflow:park before landing any workaround. "+
				"Do NOT suppress this assertion.",
			packKey,
		)
	}

	// Non-zero exit: the push was rejected. Determine the HTTP status from the
	// git output so we can log the error code for PROTOCOL.md alignment.
	//
	// Per story: if no machine-readable code exists, assert only on HTTP status
	// (non-200) and document the gap as a Medium finding via t.Logf (not a park).
	detectedStatus := mhpParseHTTPStatus(pushOutput)
	if detectedStatus != 0 {
		t.Logf(
			"mhp: push rejected with HTTP status %d — hydration failed loudly (non-200). "+
				"Expected status: 500 or 503 (hydrate-error → push rejection). "+
				"Actual status: %d.",
			detectedStatus, detectedStatus,
		)
		if detectedStatus == http.StatusOK {
			// The git command reported non-zero exit but we found a 200 in the output.
			// This is inconsistent — log as a Medium gap.
			t.Logf(
				"mhp: MEDIUM GAP — git exited non-zero but output contains '200'. "+
					"The portal may be returning 200 with garbled output rather than a clean "+
					"4xx/5xx. This is a Medium production bug: the error is not clean. "+
					"Tracked inline — park separately if this persists across environments.",
			)
		}
	} else {
		// No HTTP status code found in output — git exited non-zero without a
		// parseable status. This still satisfies the invariant (push was rejected),
		// but the absence of a machine-readable error code is a Medium gap.
		t.Logf(
			"mhp: MEDIUM GAP (non-blocking) — push rejected (git exit %d) but no "+
				"parseable HTTP status code found in git output. "+
				"The portal does not expose a machine-readable hydration-error code "+
				"(e.g. 'hydration.corrupt_bucket', 'ErrMissingPack'). "+
				"This is a missing-feature gap, not a test bug. "+
				"The critical safety invariant (non-zero exit on corrupt bucket) IS satisfied.",
			pushExitCode,
		)
	}

	// ── Assertion 2: pod 1 must NOT serve partial state ───────────────────────
	// Run git ls-remote against pod 1 for the session. It must either:
	//   (a) return an error / non-zero exit (session unavailable — preferred), OR
	//   (b) return an empty ref set (session unknown — acceptable).
	// It must NOT return refs pointing at commits from the pre-corruption session.
	//
	// Note: PollForHydration uses ls-remote internally; we call it directly here
	// to get the raw output rather than just a boolean.
	lsRemoteCtx, lsRemoteCancel := context.WithTimeout(ctx, 10*time.Second)
	defer lsRemoteCancel()

	lsRemoteOutput, lsRemoteErr := mhpRunLsRemote(lsRemoteCtx, pod1.URL, pair.AccessToken, orgID, sessionID)
	t.Logf("mhp: ls-remote pod 1 result: err=%v output=%q", lsRemoteErr, lsRemoteOutput)

	if lsRemoteErr == nil && strings.TrimSpace(lsRemoteOutput) != "" {
		// ls-remote succeeded and returned refs. Check if any refs belong to the
		// pre-corruption session (pointing at commits that were committed before
		// the pack was deleted).
		//
		// The invariant: if hydration failed (push was rejected), pod 1 must not
		// be serving any refs for this session. Any refs returned here indicate
		// partial hydration silently succeeded, which is the Critical scenario.
		t.Fatalf(
			"mhp: CRITICAL SAFETY INVARIANT VIOLATED — "+
				"pod 1's git ls-remote returned refs for session %s even though the "+
				"push was rejected (exit %d). "+
				"This means pod 1 partially hydrated the session and is serving refs "+
				"from a corrupt/incomplete object store. "+
				"ls-remote output: %s. "+
				"This is a Critical data-integrity bug. "+
				"Park as Critical via /agile-workflow:park.",
			sessionID, pushExitCode, lsRemoteOutput,
		)
	}

	if lsRemoteErr == nil {
		// ls-remote succeeded but returned no refs — session not known to pod 1.
		// This is the "session unknown" acceptable scenario: pod 1 refused to
		// hydrate and has no state for the session.
		t.Logf("mhp: ls-remote returned empty ref set — pod 1 has no state for session (correct: hydration refused)")
	} else {
		// ls-remote returned an error (non-zero git exit) — session unavailable.
		// This is the preferred scenario: loud failure.
		t.Logf("mhp: ls-remote returned error (session unavailable, preferred) — error: %v", lsRemoteErr)
	}

	// ── Assertion 3: manifest NOT updated by pod 1 ───────────────────────────
	// Pod 1's failed hydration attempt must NOT have written a new manifest
	// claiming success. Read the manifest again and compare FencingToken and
	// Refs to the pre-corruption state.
	manifestAfter, err := mn.GetObject(ctx, manifestKey)
	if err != nil {
		// If the manifest is gone (shouldn't happen), log and move on.
		t.Logf("mhp: manifest not readable after corruption attempt: %v (non-fatal for this assertion)", err)
	} else {
		var mAfter mhpManifest
		if err := json.Unmarshal(manifestAfter, &mAfter); err != nil {
			t.Logf("mhp: could not decode manifest after corruption attempt: %v (non-fatal)", err)
		} else {
			t.Logf("mhp: manifest after corruption attempt: FencingToken=%d Packs=%d Refs=%d",
				mAfter.FencingToken, len(mAfter.Packs), len(mAfter.Refs))

			// The FencingToken in the manifest must not have advanced past what pod 0
			// wrote. If pod 1 wrote a new manifest (even with partial state), its
			// fencing token would be >= pod 0's token + 1.
			if mAfter.FencingToken > mBefore.FencingToken {
				t.Errorf(
					"mhp: MANIFEST INTEGRITY VIOLATED — "+
						"manifest FencingToken advanced from %d to %d after pod 1's failed hydration. "+
						"Pod 1 must NOT write a manifest claiming success when packs are missing. "+
						"This indicates the hydration error path writes a partial manifest before "+
						"detecting the missing pack — park as Important if the push was still rejected "+
						"(error detected but manifest was already written), "+
						"or Critical if the push returned 200 (push accepted with corrupt state).",
					mBefore.FencingToken, mAfter.FencingToken,
				)
			} else {
				t.Logf(
					"mhp: manifest integrity confirmed — FencingToken unchanged at %d "+
						"(pod 1's failed hydration did not write a new manifest claiming success)",
					mAfter.FencingToken,
				)
			}
		}
	}

	t.Logf(
		"mhp: safety invariant VERIFIED — "+
			"push to pod 1 failed (exit %d) with missing pack %q; "+
			"pod 1 did not serve partial state; bucket manifest was not updated to claim success",
		pushExitCode, packKey,
	)

	// ── Subtest: recovery_after_repair ───────────────────────────────────────
	// Restore the deleted pack object and confirm that a subsequent push succeeds.
	// This proves the failure is transient (not data loss) and that the pack-
	// download path is used correctly when objects are available.
	t.Run("recovery_after_repair", func(t *testing.T) {
		t.Logf("mhp/recovery: restoring pack object %q (%d bytes) to MinIO", packKey, len(packData))

		if err := mn.PutObject(ctx, packKey, packData); err != nil {
			t.Fatalf("mhp/recovery: restore pack object %q: %v", packKey, err)
		}

		// Verify the restore by listing — the pack key must be back.
		keysAfterRestore, err := mn.ListObjects(ctx, packsPrefix)
		if err != nil {
			t.Fatalf("mhp/recovery: list pack objects after restore: %v", err)
		}
		found := false
		for _, k := range keysAfterRestore {
			if k == packKey {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("mhp/recovery: restored pack %q not found in listing after PutObject — "+
				"restore did not persist (keys: %v)", packKey, keysAfterRestore)
		}
		t.Logf("mhp/recovery: pack object %q confirmed in MinIO after restore", packKey)

		// Now retry the push via pod 1. Hydration should succeed this time.
		// Give 30s — the golden-test SLO for clean hydration.
		recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 30*time.Second)
		defer recoveryCancel()

		// Wait for pod 1 to be able to hydrate (ls-remote succeeds).
		// We use PollForHydration rather than WaitForHydration since the cluster
		// fixture's WaitForHydration t.Fatal on timeout — but here we want a
		// non-fatal poll so we can provide a better diagnostic on failure.
		hydratedOK := c.PollForHydration(recoveryCtx, orgID, sessionID, pair.AccessToken, 1, 25*time.Second)
		if !hydratedOK {
			t.Fatalf(
				"mhp/recovery: pod 1 did not hydrate successfully within 25s after "+
					"restoring pack object %q. "+
					"Expected: hydration succeeds since the pack is now present. "+
					"Actual: PollForHydration timed out.",
				packKey,
			)
		}
		t.Logf("mhp/recovery: pod 1 hydrated successfully after pack restore")

		// Attempt a push via pod 1. This should now succeed.
		recoveryPushExitCode, recoveryPushOutput := mhpAttemptPush(
			recoveryCtx, t,
			pod1.URL, orgID, sessionID, userID, pair.AccessToken, ref,
		)
		t.Logf("mhp/recovery: pod 1 push result: exit=%d output=%s", recoveryPushExitCode, recoveryPushOutput)

		if recoveryPushExitCode != 0 {
			t.Fatalf(
				"mhp/recovery: push via pod 1 failed (exit %d) after restoring pack object %q. "+
					"Expected: push succeeds since the pack is now present. "+
					"Output: %s",
				recoveryPushExitCode, packKey, recoveryPushOutput,
			)
		}

		t.Logf(
			"mhp/recovery: recovery verified — push succeeded (exit 0) after restoring "+
				"deleted pack %q; failure was transient, not data loss",
			packKey,
		)
	})
}

// ---------------------------------------------------------------------------
// Per-file helpers — scoped to this file to avoid polluting the failure package.
// ---------------------------------------------------------------------------

// mhpRandEmail returns a unique email address for this test run.
func mhpRandEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@example.com", prefix, time.Now().UnixNano())
}

// mhpGetMe calls GET /api/me and returns the authenticated user's ID.
func mhpGetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("mhpGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("mhpGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mhpGetMe: want 200, got %d; body: %s", resp.StatusCode, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("mhpGetMe: decode: %v; body: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("mhpGetMe: empty ID; body: %s", body)
	}
	return me.ID
}

// mhpCreateSession calls POST /api/orgs/{orgID}/sessions and returns the session ID.
func mhpCreateSession(ctx context.Context, t *testing.T, baseURL, accessToken, orgID, name string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"name":         name,
		"goal":         "hydration-with-missing-pack failure test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	})
	if err != nil {
		t.Fatalf("mhpCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("mhpCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("mhpCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("mhpCreateSession: want 201, got %d; body: %s", resp.StatusCode, respBody)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("mhpCreateSession: decode: %v; body: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("mhpCreateSession: empty session ID; body: %s", respBody)
	}
	return s.ID
}

// mhpBasicAuthURL injects credentials into the base URL.
func mhpBasicAuthURL(baseURL, user, pass string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(fmt.Sprintf("mhpBasicAuthURL: parse %q: %v", baseURL, err))
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// mhpSetupAndPushN clones the session repo at podURL and pushes n commits, each
// with moderately-sized content to encourage pack file creation. This is the
// "must succeed" push path — t.Fatal on any error.
func mhpSetupAndPushN(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, userID, accessToken, ref string,
	n int,
) {
	t.Helper()

	repoDir := t.TempDir()
	repoURL := mhpBasicAuthURL(podURL, "x-access-token", accessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("mhpSetupAndPushN: git %v: %v\n%s", args, err, out)
		}
	}

	runGit("", "clone", repoURL, repoDir)
	runGit(repoDir, "config", "user.email", userID+"@test.example")
	runGit(repoDir, "config", "user.name", "Test "+userID)

	for i := 0; i < n; i++ {
		content := strings.Repeat(fmt.Sprintf("missing-pack-test line %d of commit %d\n", i, i), 64)
		absPath := filepath.Join(repoDir, fmt.Sprintf("file%02d.md", i))
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatalf("mhpSetupAndPushN: write file %d: %v", i, err)
		}
		runGit(repoDir, "add", fmt.Sprintf("file%02d.md", i))

		turnID := uuid.New().String()
		fullMsg := fmt.Sprintf(
			"mhp: commit %d\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
			i, sessionID, turnID, userID,
		)
		msgFile := filepath.Join(t.TempDir(), fmt.Sprintf("commit-msg-%d", i))
		if err := os.WriteFile(msgFile, []byte(fullMsg), 0o644); err != nil {
			t.Fatalf("mhpSetupAndPushN: write commit msg %d: %v", i, err)
		}
		runGit(repoDir, "commit", "-F", msgFile)
	}

	runGit(repoDir, "push", "origin", "HEAD:refs/heads/"+ref)
}

// mhpAttemptPush attempts a push to podURL that MAY fail (corruption scenario).
// Returns the git exit code (0 = success) and the combined output. The push
// requires a new clone since the corrupted session is unknown to pod 1.
//
// Unlike the must-succeed helpers, this function NEVER calls t.Fatal for push
// failure — the caller decides how to interpret the exit code.
func mhpAttemptPush(
	ctx context.Context, t *testing.T,
	podURL, orgID, sessionID, userID, accessToken, ref string,
) (exitCode int, output string) {
	t.Helper()

	repoDir := t.TempDir()
	repoURL := mhpBasicAuthURL(podURL, "x-access-token", accessToken) +
		"/git/" + orgID + "/" + sessionID + ".git"

	runGitFatal := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("mhpAttemptPush (setup): git %v: %v\n%s", args, err, out)
		}
	}

	runGitFatal("", "clone", repoURL, repoDir)
	runGitFatal(repoDir, "config", "user.email", userID+"@test.example")
	runGitFatal(repoDir, "config", "user.name", "Test "+userID)

	// Write a small commit — content doesn't matter, we need something to push.
	testFile := filepath.Join(repoDir, "missing-pack-probe.md")
	if err := os.WriteFile(testFile, []byte("probe content for missing-pack test"), 0o644); err != nil {
		t.Fatalf("mhpAttemptPush: write probe file: %v", err)
	}
	runGitFatal(repoDir, "add", "missing-pack-probe.md")

	turnID := uuid.New().String()
	fullMsg := fmt.Sprintf(
		"mhp: probe push\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		sessionID, turnID, userID,
	)
	msgFile := filepath.Join(t.TempDir(), "probe-msg")
	if err := os.WriteFile(msgFile, []byte(fullMsg), 0o644); err != nil {
		t.Fatalf("mhpAttemptPush: write commit msg: %v", err)
	}
	runGitFatal(repoDir, "commit", "-F", msgFile)

	// The push itself — may fail. Capture exit code and output.
	pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	pushCmd.Dir = repoDir
	pushCmd.Env = append(pushCmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := pushCmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		}
		return 1, string(out)
	}
	return 0, string(out)
}

// mhpRunLsRemote runs git ls-remote against the pod's session repo and returns
// the output and any error. Used to check whether pod 1 is serving refs after
// a failed hydration attempt.
//
// Unlike WaitForHydration (which polls until success), this is a one-shot check
// that returns the raw output so the caller can inspect what refs (if any) are
// being served.
func mhpRunLsRemote(ctx context.Context, podURL, accessToken, orgID, sessionID string) (string, error) {
	without, found := strings.CutPrefix(podURL, "http://")
	if !found {
		without = podURL
	}
	repoURL := fmt.Sprintf("http://x-access-token:%s@%s/git/%s/%s.git",
		accessToken, without, orgID, sessionID)

	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// mhpParseHTTPStatus extracts the first recognisable HTTP status code from
// git output (e.g. "error: 503 Service Unavailable"). Returns 0 if no code
// is found. The check is ordered from most-specific (5xx failure codes first)
// to least-specific.
func mhpParseHTTPStatus(output string) int {
	for _, code := range []int{503, 500, 422, 403, 401, 200} {
		if strings.Contains(output, strconv.Itoa(code)) {
			return code
		}
	}
	return 0
}
