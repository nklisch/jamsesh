// Package chaos_test contains chaos-engineering scenarios for the jamsesh
// portal. Each scenario injects a failure and asserts the system's resilience
// invariant.
//
// Scenarios in this file:
//   - automerger_pause: pauses the portal container mid-merge for 5 seconds via
//     `docker pause` and asserts the merge completes after resume with no
//     spurious conflict.detected event.
//   - clock_skew_token_expiry: advances the portal's process-global clock past
//     the access-token TTL via the build-tag-gated /test/clock-advance endpoint
//     and asserts that subsequent bearer-authenticated requests return 401
//     auth.expired_token.
package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

func TestRuntimeAndClock(t *testing.T) {
	t.Run("automerger_pause", testAutomergerPause)

	// Ordering invariant: clock_skew_token_expiry mutates the portal's
	// process-global clock (forward-only and cumulative). It runs in its
	// own fresh portal+postgres+mailhog stack so the offset cannot leak
	// into any other subtest. If a future clock-sensitive subtest is
	// added to TestRuntimeAndClock, either spin up its own stack or
	// order it before this one.
	t.Run("clock_skew_token_expiry", testClockSkewTokenExpiry)
}

// errorEnvelope mirrors the portal's PROTOCOL.md error envelope shape so
// the subtest can assert on the typed code (not the human-readable message).
type errorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// testClockSkewTokenExpiry verifies the resilience invariant:
//
//	Invariant: after the portal's clock is advanced past the access-token
//	TTL, a previously-valid bearer token is rejected with 401
//	auth.expired_token. Before the advance the same token must succeed —
//	the baseline call establishes that the token is well-formed and the
//	stack is healthy, so the post-advance 401 is attributable to the
//	clock skew and not to a pre-existing misconfiguration.
//
// The subtest stands up a fresh portal+postgres+mailhog stack so the
// process-global clock offset cannot leak into sibling subtests.
func testClockSkewTokenExpiry(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// --- Fresh stack (clock offset stays contained) ---
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	email := randEmail(t, "skew")
	user := authflow.SignInViaMagicLink(ctx, t, p, mh, email)

	// --- BEFORE-ADVANCE BASELINE ---
	// GET /api/me with a fresh access token must succeed. This proves the
	// token is well-formed and the bearer middleware is wired correctly
	// before chaos (the clock skew) is injected.
	getMe(ctx, t, p, user.AccessToken)

	// --- INJECT CLOCK SKEW ---
	// Advance the portal's clock past the access-token TTL. The TTL is
	// 1 hour (locked in internal/portal/tokens/service.go AccessTokenTTL;
	// hardcoded here because tests/e2e/ is a separate go module without
	// a replace directive into the parent). 60 seconds of headroom avoids
	// any ms-level drift between issue time and the advance call.
	const accessTokenTTL = 1 * time.Hour
	p.AdvanceClock(ctx, t, accessTokenTTL+time.Minute)

	// --- ASSERT POST-ADVANCE: same token must now return 401 auth.expired_token ---
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
	if err != nil {
		t.Fatalf("clock_skew_token_expiry: build /me request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("clock_skew_token_expiry: GET /me after advance: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("clock_skew_token_expiry: GET /me after advance: status %d (want 401): %s",
			resp.StatusCode, respBody)
	}
	var env errorEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		t.Fatalf("clock_skew_token_expiry: decode error envelope: %v\nbody: %s", err, respBody)
	}
	if env.Error != "auth.expired_token" {
		t.Fatalf("clock_skew_token_expiry: error code %q (want %q)\nbody: %s",
			env.Error, "auth.expired_token", respBody)
	}
}

// testAutomergerPause verifies the resilience invariant:
//
//	Invariant: when the portal container is paused mid-merge for 5 seconds via
//	`docker pause`, the auto-merger completes the merge after resume, emitting a
//	merge.succeeded event — and no spurious conflict.detected event fires.
//
// The scenario follows the anti-tautology pattern: a baseline push is asserted
// to complete cleanly before chaos is injected. This confirms the stack is
// healthy before the pause, so any failure after the pause is attributable to
// the chaos and not to a pre-existing misconfiguration.
func testAutomergerPause(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	// --- Stack setup ---
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	aliceEmail := randEmail(t, "alice")
	bobEmail := randEmail(t, "bob")

	alice := authflow.SignInViaMagicLink(ctx, t, p, mh, aliceEmail)
	bob := authflow.SignInViaMagicLink(ctx, t, p, mh, bobEmail)

	aliceID := getMe(ctx, t, p, alice.AccessToken).ID
	bobID := getMe(ctx, t, p, bob.AccessToken).ID

	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Chaos Pause Org")
	sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "Chaos Pause Session")

	// Bob must be an org member before he can join a session.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, orgInviteID, orgInviteToken)

	sessionInviteID := inviteToSession(ctx, t, p, alice.AccessToken, orgID, sessionID, bobEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, bobEmail)
	acceptSessionInvite(ctx, t, p, bob.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// Subscribe to the session WebSocket before any pushes so events aren't missed.
	aliceWS := wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)
	bobWS := wsclient.Connect(ctx, t, p.URL, sessionID, bob.AccessToken)

	// --- Seed the session with a base commit ---
	// The auto-merger requires a `jam/<session>/draft` ref to exist before it
	// can merge sync refs. Push to the base ref (jam/<session>/base) to seed
	// draft. This push must arrive before any sync-ref pushes.
	aliceRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, aliceID, alice.AccessToken)
	baseRef := fmt.Sprintf("jam/%s/base", sessionID)
	aliceRepo.Commit(ctx, t, "base.md", "session base", "seed: base commit")
	aliceRepo.Push(ctx, t, baseRef)

	// Bob clones after the base push and resets to the base commit so his
	// commits share a common ancestor with draft (required by the auto-merger).
	bobRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, bobID, bob.AccessToken)
	gitResetToRef(ctx, t, bobRepo.Dir, "origin/"+baseRef, bobID)

	// --- BEFORE-CHAOS BASELINE ---
	// Invariant: without chaos, Alice's first sync-ref push produces a
	// merge.succeeded event within 10 seconds. This proves the stack is healthy
	// before the pause is injected.
	aliceRef := fmt.Sprintf("jam/%s/%s/main", sessionID, aliceID)
	aliceSHA1 := aliceRepo.Commit(ctx, t, "alice-a.md", "Alice: baseline", "Alice: commit A (baseline)")
	aliceRepo.Push(ctx, t, aliceRef)

	waitForMergeSucceeded(t, aliceWS, aliceSHA1, 10*time.Second)
	waitForMergeSucceeded(t, bobWS, aliceSHA1, 10*time.Second)

	// --- GET PORTAL CONTAINER NAME ---
	containerName := p.ContainerName(ctx)

	// Register a defensive unpause as the very first cleanup so the container
	// is never left paused if any subsequent step (including the test itself)
	// panics or calls t.Fatal. Cleanup runs in LIFO order — this is registered
	// early so it fires last (after closer cleanups), but for safety it fires
	// even if ContainerName returns "".
	if containerName != "" {
		t.Cleanup(func() {
			// Best-effort: ignore errors. The container may already be running.
			_ = exec.Command("docker", "unpause", containerName).Run()
		})
	}

	// --- CHAOS: push commit B, then immediately pause the portal ---
	// The push itself must complete (it is a git transport operation to the
	// portal's HTTP endpoint). The pause targets the portal process after the
	// push bytes have been received, mid-merge. When the portal resumes after
	// 5 seconds it should complete the merge and emit merge.succeeded.
	aliceSHA2 := aliceRepo.Commit(ctx, t, "alice-b.md", "Alice: chaos", "Alice: commit B (pause chaos)")

	// Push commit B. This sends the packfile to the portal; the pre-receive hook
	// validates and the auto-merger is triggered asynchronously. We pause
	// immediately after to catch the merger mid-flight.
	aliceRepo.Push(ctx, t, aliceRef)

	if containerName == "" {
		t.Skip("portal container name unavailable — cannot inject docker pause chaos")
	}

	// Pause the portal container for 5 seconds.
	pauseDuration := 5 * time.Second
	if err := exec.CommandContext(ctx, "docker", "pause", containerName).Run(); err != nil {
		// If we cannot pause (e.g., non-Linux CI without cgroups), skip.
		t.Skipf("docker pause %s: %v — skipping chaos scenario", containerName, err)
	}

	// Wait, then unpause. Using a goroutine-safe sleep so the unpause is
	// deterministic relative to the pause.
	time.Sleep(pauseDuration)

	if err := exec.CommandContext(ctx, "docker", "unpause", containerName).Run(); err != nil {
		t.Fatalf("docker unpause %s: %v — container may be stuck paused", containerName, err)
	}

	// --- ASSERT POST-CHAOS: merge.succeeded must arrive within 10s ---
	// The auto-merger should resume and complete the in-flight merge for
	// commit B after the container is unpaused.
	waitForMergeSucceeded(t, aliceWS, aliceSHA2, 10*time.Second)
	waitForMergeSucceeded(t, bobWS, aliceSHA2, 10*time.Second)

	// --- ASSERT: no spurious conflict.detected event fires ---
	// After a pause+resume the auto-merger must not emit a conflict.detected
	// event. We watch the event stream for a short window (2s) after the
	// merge.succeeded and fail if a conflict.detected slips through.
	assertNoConflictDetected(t, aliceWS, 2*time.Second)
	assertNoConflictDetected(t, bobWS, 2*time.Second)

	// Belt-and-suspenders: fetch and verify commit B is reachable from draft.
	bobRepo.Fetch(ctx, t)
	draftRef := fmt.Sprintf("jam/%s/draft", sessionID)
	draftSHA := bobRepo.RevParse(ctx, t, draftRef)
	if draftSHA == "" {
		t.Fatal("draft ref is empty after chaos auto-merge")
	}
	requireCommitInLog(t, bobRepo.Dir, draftSHA, aliceSHA2,
		"Alice's commit B must be reachable from draft after pause+resume")
}

// assertNoConflictDetected reads the event stream for window duration and
// fails the test if any conflict.detected event arrives. All other events
// (e.g., additional merge.succeeded for concurrent merges) are silently
// discarded.
func assertNoConflictDetected(t *testing.T, ws *wsclient.Client, window time.Duration) {
	t.Helper()
	deadline := time.After(window)
	for {
		select {
		case ev, ok := <-ws.Events():
			if !ok {
				// Channel closed — no conflict.detected arrived.
				return
			}
			if ev.Type == "conflict.detected" {
				t.Errorf("spurious conflict.detected event after portal pause+resume: %+v", ev)
				return
			}
			// Discard other events and keep watching.
		case <-deadline:
			// Window elapsed with no conflict.detected — the invariant holds.
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Local types and helpers.
// randEmail and requireDocker/requirePortalImage live in
// network_and_provider_test.go (same chaos_test package); they are reused here.
// ---------------------------------------------------------------------------

// chaosSessionRef mirrors the JSON returned by POST /api/orgs/{orgID}/sessions.
type chaosSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// chaosInviteRef mirrors the minimal Invite JSON we need.
type chaosInviteRef struct {
	ID string `json:"id"`
}

// chaosMeResponse mirrors the minimal /me response.
type chaosMeResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// chaosMergeSucceededPayload mirrors the portal's merge.succeeded event payload.
type chaosMergeSucceededPayload struct {
	SourceSha      string `json:"source_sha"`
	DraftSha       string `json:"draft_sha"`
	MergeCommitSha string `json:"merge_commit_sha"`
}

// getMe calls GET /api/me and returns the caller's user record.
func getMe(ctx context.Context, t *testing.T, p *portal.Portal, accessToken string) chaosMeResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
	if err != nil {
		t.Fatalf("getMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getMe: GET /me: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getMe: status %d: %s", resp.StatusCode, respBody)
	}
	var me chaosMeResponse
	if err := json.Unmarshal(respBody, &me); err != nil {
		t.Fatalf("getMe: decode: %v\nbody: %s", err, respBody)
	}
	return me
}

// createSession calls POST /api/orgs/{orgID}/sessions and returns the new
// session's ID.
func createSession(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "Chaos e2e session",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("createSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", p.URL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("createSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createSession: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var s chaosSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("createSession: decode: %v\nbody: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("createSession: empty id in response")
	}
	return s.ID
}

// inviteToSession calls POST /api/orgs/{orgID}/sessions/{sessionID}/invites and
// returns the new invite's ID.
func inviteToSession(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, email string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/invites", p.URL, orgID, sessionID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("inviteToSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("inviteToSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("inviteToSession: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var inv chaosInviteRef
	if err := json.Unmarshal(respBody, &inv); err != nil {
		t.Fatalf("inviteToSession: decode: %v\nbody: %s", err, respBody)
	}
	if inv.ID == "" {
		t.Fatalf("inviteToSession: empty id in response")
	}
	return inv.ID
}

// extractSessionInviteToken polls MailHog for a session invite email and
// returns the raw token.
func extractSessionInviteToken(ctx context.Context, t *testing.T, mh *mailhog.MailHog, email string) string {
	t.Helper()
	msg := mh.LatestMessageTo(ctx, t, email, 10*time.Second)
	body := authflow.DecodeEmailBody(msg.Body)
	matches := authflow.InviteTokenRE.FindStringSubmatch(body)
	if len(matches) < 2 {
		t.Fatalf("extractSessionInviteToken(%s): could not find token in email body:\n%s", email, body)
	}
	return matches[1]
}

// acceptSessionInvite calls POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept.
func acceptSessionInvite(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, inviteID, token string) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/invites/%s/accept", p.URL, orgID, sessionID, inviteID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("acceptSessionInvite: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("acceptSessionInvite: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("acceptSessionInvite: status %d (want 200): %s", resp.StatusCode, respBody)
	}
}

// gitResetToRef runs `git fetch origin && git reset --hard <remoteRef>` in
// repoDir, then re-configures git identity so subsequent commits don't fail.
func gitResetToRef(ctx context.Context, t *testing.T, repoDir, remoteRef, userID string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gitResetToRef: git %v: %v\n%s", args, err, out)
		}
	}
	run("fetch", "origin")
	run("reset", "--hard", remoteRef)
	run("config", "user.email", userID+"@test.example")
	run("config", "user.name", "Test "+userID)
}

// waitForMergeSucceeded drains ws until a merge.succeeded event whose
// SourceSha matches wantSHA arrives, or until timeout expires.
func waitForMergeSucceeded(t *testing.T, ws *wsclient.Client, wantSHA string, timeout time.Duration) {
	t.Helper()
	shortSHA := wantSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ws.Events():
			if !ok {
				t.Fatalf("waitForMergeSucceeded(%s): event channel closed before merge.succeeded arrived", shortSHA)
				return
			}
			if ev.Type != "merge.succeeded" {
				continue
			}
			var p chaosMergeSucceededPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				t.Logf("waitForMergeSucceeded(%s): ignoring unparseable payload: %v", shortSHA, err)
				continue
			}
			if strings.HasPrefix(p.SourceSha, wantSHA) || strings.HasPrefix(wantSHA, p.SourceSha) {
				return
			}
		case <-deadline:
			t.Fatalf("waitForMergeSucceeded(%s): timed out after %s waiting for merge.succeeded event",
				shortSHA, timeout)
		}
	}
}

// requireCommitInLog asserts that commitSHA is reachable from tipSHA in git
// history using `git merge-base --is-ancestor`.
func requireCommitInLog(t *testing.T, repoDir, tipSHA, commitSHA, msg string) {
	t.Helper()
	cmd := exec.Command("git", "merge-base", "--is-ancestor", commitSHA, tipSHA)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("requireCommitInLog: %s is not reachable from %s: %v\n--- %s",
			commitSHA[:7], tipSHA[:7], err, msg)
	}
}
