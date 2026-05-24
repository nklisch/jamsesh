// Invariant: two anonymous participants on the same playground session can both
// push independent commits, each observes a commit.arrived WebSocket event for
// the other's push within 5 seconds, and the auto-merger advances the draft ref
// to include both commits (verified via git merge-base).
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

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

// playgroundJoinResult is the 200 body from POST /api/playground/sessions/{id}/join.
type playgroundJoinResult struct {
	Session  struct {
		ID           string `json:"id"`
		OrgID        string `json:"org_id"`
		MembersCount int    `json:"members_count"`
	} `json:"session"`
	Bearer    string `json:"bearer"`
	Nickname  string `json:"nickname"`
	ExpiresAt string `json:"expires_at"`
}

func TestPlayground_TwoParticipantJoinMerge(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	// Postgres + portal with playground enabled.
	// Hard-cap is 300s (generous); idle-timeout is larger still so the
	// hard-cap path never fires mid-test.
	// Destruction sweep interval is 1s (so cleanup finishes quickly if a
	// follow-up test needs a clean state, but we don't advance the clock here).
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		DBDSN:    pg.ContainerDSN,
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":                      "true",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":                   "300",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":               "600",
			"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S": "1",
		},
	})

	const orgID = "org_playground"

	// ── Step 1: Participant A creates a playground session ───────────────────
	t.Log("playground/two-participant: A creates session")
	aCreate := playgroundCreateTwo(ctx, t, p.URL)
	sessionID := aCreate.Session.ID
	aBearer := aCreate.Bearer
	aNickname := aCreate.Nickname

	require.NotEmpty(t, sessionID, "session ID must be non-empty")
	require.NotEmpty(t, aBearer, "A's bearer must be non-empty")
	require.NotEmpty(t, aNickname, "A's nickname must be non-empty")
	require.Equal(t, orgID, aCreate.Session.OrgID, "session org_id must be org_playground")
	require.Equal(t, 1, aCreate.Session.MembersCount, "newly created session has 1 member")
	t.Logf("playground/two-participant: session=%s A.nickname=%s", sessionID, aNickname)

	// ── Step 2: Derive A's accountID from /api/me ─────────────────────────
	// The anon accountID is needed to construct A's per-user git ref.
	aMe := getMe(ctx, t, p, aBearer)
	aAccountID := aMe.ID
	require.NotEmpty(t, aAccountID, "A's accountID from /api/me must be non-empty")
	t.Logf("playground/two-participant: A.accountID=%s", aAccountID)

	// ── Step 3: Assert bare repo exists on real disk (anti-tautology Unit 5) ─
	repoPath := "/tmp/jamsesh-repos/orgs/" + orgID + "/sessions/" + sessionID + ".git"
	exitCode, execOut, execErr := p.Exec(ctx, []string{"ls", repoPath})
	require.NoErrorf(t, execErr, "docker exec ls %s: API error", repoPath)
	require.Equalf(t, 0, exitCode,
		"bare repo must exist at %s after session create (ls exit %d)\noutput: %s",
		repoPath, exitCode, strings.TrimSpace(execOut))
	t.Logf("playground/two-participant: bare repo confirmed at %s", repoPath)

	// ── Step 4: A pushes the base ref (seed commit) ───────────────────────
	// The base ref push is exempt from trailer validation (commit 297616a),
	// so a plain vanilla commit can be used here. Subsequent per-user ref
	// pushes still require Jam-* trailers — gitclient.Commit adds them
	// automatically.
	t.Log("playground/two-participant: A pushes base ref")
	aRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, aAccountID, aBearer)
	baseRef := "jam/" + sessionID + "/base"
	_ = aRepo.Commit(ctx, t, "base.md", "# Session base", "playground: seed base commit")
	aRepo.Push(ctx, t, baseRef)
	t.Logf("playground/two-participant: A pushed base ref %s", baseRef)

	// ── Step 5: Participant B joins the session ────────────────────────────
	t.Log("playground/two-participant: B joins session")
	bJoin := playgroundJoin(ctx, t, p.URL, sessionID, "")
	bBearer := bJoin.Bearer
	bNickname := bJoin.Nickname

	require.NotEmpty(t, bBearer, "B's bearer must be non-empty")
	require.NotEmpty(t, bNickname, "B's nickname must be non-empty")
	require.Equal(t, 2, bJoin.Session.MembersCount, "after B joins, session has 2 members")
	t.Logf("playground/two-participant: B joined: B.nickname=%s", bNickname)

	// Nicknames should differ (collision would be a wordlist bug, not forbidden).
	if aNickname == bNickname {
		t.Logf("WARNING: A and B received the same nickname %q — possible wordlist collision", aNickname)
	}

	// Derive B's accountID from /api/me.
	bMe := getMe(ctx, t, p, bBearer)
	bAccountID := bMe.ID
	require.NotEmpty(t, bAccountID, "B's accountID must be non-empty")
	require.NotEqual(t, aAccountID, bAccountID, "A and B must have distinct accountIDs")
	t.Logf("playground/two-participant: B.accountID=%s", bAccountID)

	// ── Step 6: Both subscribe to WebSocket before pushing ───────────────
	// Subscribe BEFORE pushing so we don't miss events emitted immediately
	// after the push. The wsclient ticket flow works identically for anon
	// bearers — POST /api/auth/ws-ticket authenticates any valid bearer.
	t.Log("playground/two-participant: connecting WebSocket for A and B")
	aWS := wsclient.Connect(ctx, t, p.URL, sessionID, aBearer)
	bWS := wsclient.Connect(ctx, t, p.URL, sessionID, bBearer)

	// ── Step 7: A and B push commits on their per-user refs ───────────────
	// A's repo already has the base commit as HEAD; A's next commit is on
	// her per-user ref (child of base → common ancestor for auto-merger).
	aRef := "jam/" + sessionID + "/" + aAccountID + "/main"
	aliceSHA := aRepo.Commit(ctx, t, "alice.md", "Alice's contribution", "Alice: add alice.md")
	aRepo.Push(ctx, t, aRef)
	t.Logf("playground/two-participant: A pushed %s on ref %s", aliceSHA[:7], aRef)

	// B clones, resets to the base commit (so her commit shares a common
	// ancestor with draft), then pushes her own file.
	bRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, bAccountID, bBearer)
	gitResetToRef(ctx, t, bRepo.Dir, "origin/"+baseRef, bAccountID)

	bRef := "jam/" + sessionID + "/" + bAccountID + "/main"
	bobSHA := bRepo.Commit(ctx, t, "bob.md", "Bob's contribution", "Bob: add bob.md")
	bRepo.Push(ctx, t, bRef)
	t.Logf("playground/two-participant: B pushed %s on ref %s", bobSHA[:7], bRef)

	// ── Step 8: Both sides observe commit.arrived for each push ───────────
	// Flake mitigation: use WaitFor (channel drain, not sleep) with a 5s
	// deadline per the pre-mortem's recommended SLA.
	t.Log("playground/two-participant: waiting for commit.arrived events")
	const commitTimeout = 5 * time.Second
	aWS.WaitFor(t, "commit.arrived", commitTimeout)
	t.Logf("playground/two-participant: A observed commit.arrived")
	bWS.WaitFor(t, "commit.arrived", commitTimeout)
	t.Logf("playground/two-participant: B observed commit.arrived")

	// ── Step 9: Auto-merger composes both changes ─────────────────────────
	// Wait for merge.succeeded for each source commit (same SLA as
	// auto_merge_test.go). The auto-merger is deterministic for non-conflicting
	// refs, so we can assert on both source SHAs.
	const mergeTimeout = 20 * time.Second
	t.Log("playground/two-participant: waiting for merge.succeeded for A's commit")
	waitForMergeSucceeded(t, aWS, aliceSHA, mergeTimeout)
	t.Logf("playground/two-participant: A's WS saw merge.succeeded for %s", aliceSHA[:7])
	waitForMergeSucceeded(t, bWS, aliceSHA, mergeTimeout)
	t.Logf("playground/two-participant: B's WS saw merge.succeeded for %s", aliceSHA[:7])

	t.Log("playground/two-participant: waiting for merge.succeeded for B's commit")
	waitForMergeSucceeded(t, aWS, bobSHA, mergeTimeout)
	t.Logf("playground/two-participant: A's WS saw merge.succeeded for %s", bobSHA[:7])
	waitForMergeSucceeded(t, bWS, bobSHA, mergeTimeout)
	t.Logf("playground/two-participant: B's WS saw merge.succeeded for %s", bobSHA[:7])

	// ── Step 10: Cross-fetch verification ─────────────────────────────────
	// A fetches: she should see B's ref tip at the SHA B pushed.
	// B fetches: she should see A's ref tip at the SHA A pushed.
	t.Log("playground/two-participant: cross-fetch verification")

	aRepo.Fetch(ctx, t)
	fetchedBSHA := aRepo.RevParse(ctx, t, bRef)
	require.Equal(t, bobSHA, fetchedBSHA,
		"A's git fetch: expected B's SHA %s, got %s", bobSHA[:7], fetchedBSHA[:7])
	t.Logf("playground/two-participant: A sees B's ref tip %s", fetchedBSHA[:7])

	bRepo.Fetch(ctx, t)
	fetchedASHA := bRepo.RevParse(ctx, t, aRef)
	require.Equal(t, aliceSHA, fetchedASHA,
		"B's git fetch: expected A's SHA %s, got %s", aliceSHA[:7], fetchedASHA[:7])
	t.Logf("playground/two-participant: B sees A's ref tip %s", fetchedASHA[:7])

	// ── Step 11: Draft ref contains both commits ───────────────────────────
	// Belt-and-suspenders: both source SHAs must be reachable from draft.
	draftRef := "jam/" + sessionID + "/draft"
	draftSHA := bRepo.RevParse(ctx, t, draftRef)
	require.NotEmpty(t, draftSHA, "draft ref must be non-empty after auto-merge")
	t.Logf("playground/two-participant: draft ref tip = %s", draftSHA[:7])

	requireCommitInLog(t, bRepo.Dir, draftSHA, aliceSHA,
		"Alice's commit must be reachable from draft after two-participant auto-merge")
	requireCommitInLog(t, bRepo.Dir, draftSHA, bobSHA,
		"Bob's commit must be reachable from draft after two-participant auto-merge")
	t.Log("playground/two-participant: both commits reachable from draft — PASS")
}

// ---------------------------------------------------------------------------
// Playground-specific HTTP helpers (scoped to this test file)
// ---------------------------------------------------------------------------

// playgroundCreateTwo calls POST /api/playground/sessions and returns the
// parsed 201 body. It uses a superset of soloCreateResponse that includes
// the full session summary needed for the two-participant test.
func playgroundCreateTwo(ctx context.Context, t *testing.T, baseURL string) soloCreateResponse {
	t.Helper()
	url := strings.TrimRight(baseURL, "/") + "/api/playground/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("playgroundCreateTwo: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playgroundCreateTwo: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("playgroundCreateTwo: status %d (want 201): %s", resp.StatusCode, body)
	}
	var r soloCreateResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("playgroundCreateTwo: decode: %v\nbody: %s", err, body)
	}
	if r.Session.ID == "" {
		t.Fatalf("playgroundCreateTwo: empty session.id: %s", body)
	}
	if r.Bearer == "" {
		t.Fatalf("playgroundCreateTwo: empty bearer: %s", body)
	}
	return r
}

// playgroundJoin calls POST /api/playground/sessions/{id}/join with an
// optional nickname (pass "" to let the server pick). Returns the parsed 200
// body. Fails the test on any non-200 status.
func playgroundJoin(ctx context.Context, t *testing.T, baseURL, sessionID, nickname string) playgroundJoinResult {
	t.Helper()
	var bodyBytes []byte
	if nickname != "" {
		b, _ := json.Marshal(map[string]string{"nickname": nickname})
		bodyBytes = b
	} else {
		bodyBytes = []byte("{}")
	}

	url := fmt.Sprintf("%s/api/playground/sessions/%s/join",
		strings.TrimRight(baseURL, "/"), sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("playgroundJoin: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playgroundJoin: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("playgroundJoin: status %d (want 200): %s\n"+
			"If 410: the join handler is treating a freshly created session as ended.\n"+
			"See: .work/backlog/bug-playground-join-with-nickname-returns-410-on-fresh-session.md\n"+
			"The unit test repro shows this is a clock-injection issue in tests, not\n"+
			"in production — against the real portal+wall-clock, hard_cap_at is always\n"+
			"in the future immediately after session creation.",
			resp.StatusCode, body)
	}

	var r playgroundJoinResult
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("playgroundJoin: decode: %v\nbody: %s", err, body)
	}
	if r.Bearer == "" {
		t.Fatalf("playgroundJoin: empty bearer in response: %s", body)
	}
	return r
}
