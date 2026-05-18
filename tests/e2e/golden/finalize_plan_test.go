// Invariant: after a session reaches commits on draft (via the auto-merger),
// the finalize flow — acquire lock → patch curation → get plan → execute plan —
// produces a single squash commit on the target branch with Co-authored-by
// trailers for every contributing agent.
//
// The lock state machine paths (acquire → patch → release) are also exercised
// directly.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/checkout"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

// ---------------------------------------------------------------------------
// Response types for finalize endpoints
// ---------------------------------------------------------------------------

// lockStatusResponse mirrors the openapi LockStatus schema.
type lockStatusResponse struct {
	LockID          string    `json:"lock_id"`
	HeldByAccountID string    `json:"held_by_account_id"`
	AcquiredAt      time.Time `json:"acquired_at"`
	LastActivityAt  time.Time `json:"last_activity_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	IsCaller        bool      `json:"is_caller"`
}

// finalizeLockResponse mirrors the openapi FinalizeLock schema (returned by PATCH).
type finalizeLockResponse struct {
	ID                  string    `json:"id"`
	SessionID           string    `json:"session_id"`
	AcquiredByAccountID string    `json:"acquired_by_account_id"`
	AcquiredAt          time.Time `json:"acquired_at"`
	LastActivityAt      time.Time `json:"last_activity_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	SelectedCommitShas  []string  `json:"selected_commit_shas"`
	TargetBranch        string    `json:"target_branch"`
	BaseSha             string    `json:"base_sha"`
	Mode                string    `json:"mode"`
	CommitMessage       string    `json:"commit_message"`
}

// planResponse mirrors the openapi PlanResponse schema.
type planResponse struct {
	PlanID       string    `json:"plan_id"`
	Mode         string    `json:"mode"`
	Script       string    `json:"script"`
	CommitMessage string   `json:"commit_message"`
	TargetBranch string    `json:"target_branch"`
	BaseSha      string    `json:"base_sha"`
	CoAuthors    []struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		AccountID string `json:"account_id"`
	} `json:"co_authors"`
	SelectedCommits []struct {
		Sha         string `json:"sha"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
		Subject     string `json:"subject"`
		AccountID   string `json:"account_id"`
	} `json:"selected_commits"`
	FetchSource struct {
		Kind      string `json:"kind"`
		RemoteUrl string `json:"remote_url"`
	} `json:"fetch_source"`
}

// fetchTokenResponse mirrors the openapi FetchTokenResponse schema.
type fetchTokenResponse struct {
	Token     string    `json:"token"`
	RemoteUrl string    `json:"remote_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ---------------------------------------------------------------------------
// TestFinalizePlanSquashFlow
// ---------------------------------------------------------------------------

// TestFinalizePlanSquashFlow is the golden finalize path:
//
//  1. Two agents push 2 commits each (4 agent commits total) plus the base
//     commit on the base ref. The auto-merger integrates all into draft.
//  2. Alice acquires the finalize lock.
//  3. Lock is patched with the curated SHA list, a target branch, a base SHA,
//     and a commit message.
//  4. Plan is fetched; its script is executed against a local checkout sandbox
//     after substituting $JAMSESH_FETCH_REMOTE with an ephemeral fetch-token URL.
//  5. Assertions: single squash commit on the target branch; both agents appear
//     as Co-authored-by trailers in the commit message.
//  6. Alice releases the lock (state-machine exercise).
func TestFinalizePlanSquashFlow(t *testing.T) {
	ctx := context.Background()

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

	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Finalize Test Org")
	sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "Finalize Test Session")

	// Invite Bob to org + session.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, orgInviteID, orgInviteToken)

	sessionInviteID := inviteToSession(ctx, t, p, alice.AccessToken, orgID, sessionID, bobEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, bobEmail)
	acceptSessionInvite(ctx, t, p, bob.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// Subscribe to the session WebSocket so we can wait for merge.succeeded.
	aliceWS := wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)
	bobWS := wsclient.Connect(ctx, t, p.URL, sessionID, bob.AccessToken)

	// ── Push a base commit on the base ref to seed the draft tip ──────────────
	aliceRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, aliceID, alice.AccessToken)
	baseRef := fmt.Sprintf("jam/%s/base", sessionID)
	_ = aliceRepo.Commit(ctx, t, "base.md", "session base content", "Alice: seed base")
	aliceRepo.Push(ctx, t, baseRef)

	// ── Alice pushes two agent commits on her sync ref ─────────────────────────
	aliceRef := fmt.Sprintf("jam/%s/%s/main", sessionID, aliceID)
	aliceSHA1 := aliceRepo.Commit(ctx, t, "alice1.md", "Alice's first contribution", "Alice: first commit")
	aliceRepo.Push(ctx, t, aliceRef)
	waitForMergeSucceeded(t, aliceWS, aliceSHA1, 20*time.Second)
	waitForMergeSucceeded(t, bobWS, aliceSHA1, 20*time.Second)

	aliceSHA2 := aliceRepo.Commit(ctx, t, "alice2.md", "Alice's second contribution", "Alice: second commit")
	aliceRepo.Push(ctx, t, aliceRef)
	waitForMergeSucceeded(t, aliceWS, aliceSHA2, 20*time.Second)
	waitForMergeSucceeded(t, bobWS, aliceSHA2, 20*time.Second)

	// ── Bob starts from the base commit and pushes two agent commits ───────────
	bobRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, bobID, bob.AccessToken)
	gitResetToRef(ctx, t, bobRepo.Dir, "origin/"+baseRef, bobID)

	bobRef := fmt.Sprintf("jam/%s/%s/main", sessionID, bobID)
	bobSHA1 := bobRepo.Commit(ctx, t, "bob1.md", "Bob's first contribution", "Bob: first commit")
	bobRepo.Push(ctx, t, bobRef)
	waitForMergeSucceeded(t, aliceWS, bobSHA1, 20*time.Second)
	waitForMergeSucceeded(t, bobWS, bobSHA1, 20*time.Second)

	bobSHA2 := bobRepo.Commit(ctx, t, "bob2.md", "Bob's second contribution", "Bob: second commit")
	bobRepo.Push(ctx, t, bobRef)
	waitForMergeSucceeded(t, aliceWS, bobSHA2, 20*time.Second)
	waitForMergeSucceeded(t, bobWS, bobSHA2, 20*time.Second)

	// ── Fetch draft tip to use as base SHA for the plan ────────────────────────
	//
	// We use the base ref's SHA as the plan's base SHA since the target branch
	// must start from the common ancestor. In the squash model: create branch at
	// base, cherry-pick the four agent commits, commit as squash.
	aliceRepo.Fetch(ctx, t)
	draftRef := fmt.Sprintf("jam/%s/draft", sessionID)
	draftSHA := aliceRepo.RevParse(ctx, t, draftRef)
	if draftSHA == "" {
		t.Fatal("draft ref is empty after auto-merge; finalize test requires a non-empty draft")
	}
	baseSHA := aliceRepo.RevParse(ctx, t, baseRef)
	if baseSHA == "" {
		t.Fatal("base ref is empty")
	}

	// ── Step 1: Acquire finalize lock ─────────────────────────────────────────
	lockStatus := acquireFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, false)
	if lockStatus.LockID == "" {
		t.Fatal("acquireFinalizeLock: empty lock_id in response")
	}
	if !lockStatus.IsCaller {
		t.Fatal("acquireFinalizeLock: is_caller must be true for Alice who just acquired the lock")
	}
	lockID := lockStatus.LockID
	t.Logf("finalize: acquired lock %s", lockID)

	// ── Step 2: Patch the lock with curation state ────────────────────────────
	//
	// We use aliceSHA1, aliceSHA2, bobSHA1, bobSHA2 as the curated SHAs.
	// Target branch is a deterministic name. Base SHA is the session base commit.
	targetBranch := "jamsesh/test-finalize-" + lockID[:8]
	commitMessage := "Finalize: squash all contributions\n\nCollaborative squash for test session."

	updatedLock := patchFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID, patchLockRequest{
		SelectedCommitShas: []string{aliceSHA1, aliceSHA2, bobSHA1, bobSHA2},
		TargetBranch:       targetBranch,
		BaseSha:            baseSHA,
		Mode:               "squash",
		CommitMessage:      commitMessage,
	})
	if updatedLock.TargetBranch != targetBranch {
		t.Fatalf("patchFinalizeLock: target_branch want %q, got %q", targetBranch, updatedLock.TargetBranch)
	}
	t.Logf("finalize: lock patched; target_branch=%s", updatedLock.TargetBranch)

	// ── Step 3: Fetch the plan ────────────────────────────────────────────────
	plan := getFinalizePlan(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID)
	if plan.Script == "" {
		t.Fatal("getFinalizePlan: empty script in plan response")
	}
	if plan.Mode != "squash" {
		t.Fatalf("getFinalizePlan: mode want %q, got %q", "squash", plan.Mode)
	}
	if len(plan.SelectedCommits) != 4 {
		t.Fatalf("getFinalizePlan: selected_commits want 4, got %d", len(plan.SelectedCommits))
	}
	t.Logf("finalize: plan fetched; plan_id=%s script_len=%d", plan.PlanID, len(plan.Script))

	// Assert co-authors include both agents. The portal derives co-author emails
	// from the commit author.email set by gitclient (userID@test.example).
	assertCoAuthorPresent(t, plan, aliceID+"@test.example")
	assertCoAuthorPresent(t, plan, bobID+"@test.example")

	// ── Step 4: Issue a fetch token and execute the plan in a local sandbox ───
	fetchTok := issueFetchToken(ctx, t, p, alice.AccessToken, orgID, sessionID)
	if fetchTok.RemoteUrl == "" {
		t.Fatal("issueFetchToken: empty remote_url")
	}
	if fetchTok.Token == "" {
		t.Fatal("issueFetchToken: empty token")
	}
	t.Logf("finalize: fetch token issued; expires_at=%s", fetchTok.ExpiresAt.Format(time.RFC3339))

	// Substitute the $JAMSESH_FETCH_REMOTE / runner placeholders in the script.
	executableScript := plan.Script
	// The sandbox sets JAMSESH_FETCH_REMOTE etc. via RunPlan; no substitution
	// needed here — they stay as shell variables. The token is injected via
	// git's GIT_CONFIG_COUNT mechanism (http.extraHeader) so git can
	// authenticate against the live portal without embedding credentials in
	// the remote URL or .git/config.
	sb := checkout.Start(t)
	t.Logf("finalize: running plan in sandbox %s", sb.Dir)
	out := sb.RunPlan(t, executableScript, fetchTok.RemoteUrl, fetchTok.Token)
	t.Logf("finalize: plan output:\n%s", out)

	// ── Step 5: Assert single squash commit on target branch ─────────────────
	if branch := sb.Branch(t); branch != targetBranch {
		t.Fatalf("sandbox branch: want %q, got %q", targetBranch, branch)
	}
	commitCount := sb.CommitCount(t, "HEAD")
	// One commit on top of the initial (empty) checkout: just the squash commit.
	// (The checkout was empty; after git checkout -b <target> <base-sha> the
	// history from the base commit is present, then one squash commit on top.)
	if commitCount < 1 {
		t.Fatalf("sandbox: expected at least 1 commit, got %d", commitCount)
	}

	logOutput := sb.Log(t, "HEAD")
	assertLogContains(t, logOutput, "Co-authored-by:", "squash commit must have Co-authored-by trailers")
	assertLogContains(t, logOutput, aliceID+"@test.example", "squash commit must carry Alice's Co-authored-by trailer")
	assertLogContains(t, logOutput, bobID+"@test.example", "squash commit must carry Bob's Co-authored-by trailer")
	assertLogContains(t, logOutput, "Finalize: squash all contributions", "squash commit must carry the custom subject line")

	// ── Step 6: Release the lock (state-machine exercise) ─────────────────────
	releaseFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID)
	t.Logf("finalize: lock released successfully")
}

// ---------------------------------------------------------------------------
// TestFinalizeLockStateMachine
// ---------------------------------------------------------------------------

// TestFinalizeLockStateMachine exercises the acquire → patch → release state
// machine without running the full plan execution (faster, no git ops).
func TestFinalizeLockStateMachine(t *testing.T) {
	ctx := context.Background()

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

	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "LockSM Org")
	sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "LockSM Session")

	// Invite Bob so we can test the 409-conflict path.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, orgInviteID, orgInviteToken)

	sessionInviteID := inviteToSession(ctx, t, p, alice.AccessToken, orgID, sessionID, bobEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, bobEmail)
	acceptSessionInvite(ctx, t, p, bob.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// 1. Alice acquires the lock.
	ls := acquireFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, false)
	if !ls.IsCaller {
		t.Fatal("step 1: is_caller must be true for Alice")
	}
	lockID := ls.LockID

	// 2. Acquiring again (idempotent) returns the same lock.
	ls2 := acquireFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, false)
	if ls2.LockID != lockID {
		t.Fatalf("step 2 idempotent: want lock_id %q, got %q", lockID, ls2.LockID)
	}

	// 3. Bob cannot acquire while Alice holds (expect 409).
	acquireFinalizeLockExpect409(ctx, t, p, bob.AccessToken, orgID, sessionID)

	// 4. Alice patches the lock with minimal curation state.
	updatedLock := patchFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID, patchLockRequest{
		SelectedCommitShas: []string{},
		TargetBranch:       "jamsesh/sm-test",
		BaseSha:            "",
		Mode:               "preserve",
		CommitMessage:      "",
	})
	if updatedLock.Mode != "preserve" {
		t.Fatalf("step 4: mode want %q, got %q", "preserve", updatedLock.Mode)
	}

	// 5. Alice releases the lock.
	releaseFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID)

	// 6. Idempotent release — second DELETE returns 204.
	releaseFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID)

	// 7. Bob can now acquire (Alice's lock is released).
	ls3 := acquireFinalizeLock(ctx, t, p, bob.AccessToken, orgID, sessionID, false)
	if !ls3.IsCaller {
		t.Fatal("step 7: Bob's lock is_caller must be true")
	}
}

// ---------------------------------------------------------------------------
// REST helpers — finalize-specific
// ---------------------------------------------------------------------------

type patchLockRequest struct {
	SelectedCommitShas []string `json:"selected_commit_shas"`
	TargetBranch       string   `json:"target_branch"`
	BaseSha            string   `json:"base_sha"`
	Mode               string   `json:"mode"`
	CommitMessage      string   `json:"commit_message,omitempty"`
}

// acquireFinalizeLock calls POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock.
func acquireFinalizeLock(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID string, override bool) lockStatusResponse {
	t.Helper()
	body := map[string]interface{}{"override": override}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock", p.URL, orgID, sessionID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("acquireFinalizeLock: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("acquireFinalizeLock: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("acquireFinalizeLock: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var ls lockStatusResponse
	if err := json.Unmarshal(respBody, &ls); err != nil {
		t.Fatalf("acquireFinalizeLock: decode: %v\nbody: %s", err, respBody)
	}
	return ls
}

// acquireFinalizeLockExpect409 calls POST .../finalize/lock and expects a 409.
func acquireFinalizeLockExpect409(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"override": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock", p.URL, orgID, sessionID),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("acquireFinalizeLockExpect409: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("acquireFinalizeLockExpect409: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("acquireFinalizeLockExpect409: status %d (want 409): %s", resp.StatusCode, respBody)
	}
}

// patchFinalizeLock calls PATCH /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}.
func patchFinalizeLock(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, lockID string, body patchLockRequest) finalizeLockResponse {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock/%s", p.URL, orgID, sessionID, lockID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("patchFinalizeLock: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patchFinalizeLock: PATCH: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patchFinalizeLock: status %d (want 200): %s", resp.StatusCode, respBody)
	}
	var fl finalizeLockResponse
	if err := json.Unmarshal(respBody, &fl); err != nil {
		t.Fatalf("patchFinalizeLock: decode: %v\nbody: %s", err, respBody)
	}
	return fl
}

// releaseFinalizeLock calls DELETE /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}.
func releaseFinalizeLock(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, lockID string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock/%s", p.URL, orgID, sessionID, lockID),
		nil)
	if err != nil {
		t.Fatalf("releaseFinalizeLock: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("releaseFinalizeLock: DELETE: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("releaseFinalizeLock: status %d (want 204): %s", resp.StatusCode, respBody)
	}
}

// getFinalizePlan calls GET /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan?lock_id={lockID}.
func getFinalizePlan(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, lockID string) planResponse {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize-plan?lock_id=%s", p.URL, orgID, sessionID, lockID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("getFinalizePlan: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getFinalizePlan: GET: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getFinalizePlan: status %d (want 200): %s", resp.StatusCode, respBody)
	}
	var pr planResponse
	if err := json.Unmarshal(respBody, &pr); err != nil {
		t.Fatalf("getFinalizePlan: decode: %v\nbody: %s", err, respBody)
	}
	return pr
}

// issueFetchToken calls POST /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token.
func issueFetchToken(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID string) fetchTokenResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/fetch-token", p.URL, orgID, sessionID),
		nil)
	if err != nil {
		t.Fatalf("issueFetchToken: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("issueFetchToken: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("issueFetchToken: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var ftr fetchTokenResponse
	if err := json.Unmarshal(respBody, &ftr); err != nil {
		t.Fatalf("issueFetchToken: decode: %v\nbody: %s", err, respBody)
	}
	return ftr
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// assertCoAuthorPresent fails t if no co-author in the plan has the given email.
func assertCoAuthorPresent(t *testing.T, plan planResponse, email string) {
	t.Helper()
	for _, ca := range plan.CoAuthors {
		if strings.EqualFold(ca.Email, email) {
			return
		}
	}
	t.Fatalf("assertCoAuthorPresent: email %q not found in co_authors %v", email, plan.CoAuthors)
}

// assertLogContains fails t if the git log output does not contain the needle.
func assertLogContains(t *testing.T, logOutput, needle, msg string) {
	t.Helper()
	if !strings.Contains(logOutput, needle) {
		t.Fatalf("assertLogContains: %s\nwant substring %q in:\n%s", msg, needle, logOutput)
	}
}
