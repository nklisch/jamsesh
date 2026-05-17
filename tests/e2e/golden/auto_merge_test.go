// Invariant: when two agents push non-conflicting commits on independent sync
// refs, the portal's auto-merger advances `draft` to include both commits AND
// both agents receive a merge.succeeded WebSocket event for each source commit.
package golden_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// mergeSucceededPayload mirrors the portal's merge.succeeded event payload.
// Field names match the JSON tags on openapi.MergeSucceededPayload.
type mergeSucceededPayload struct {
	SourceSha      string `json:"source_sha"`
	DraftSha       string `json:"draft_sha"`
	MergeCommitSha string `json:"merge_commit_sha"`
}

func TestAutoMergeTwoAgents(t *testing.T) {
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

	// Both agents sign in via magic link.
	alice := authflow.SignInViaMagicLink(ctx, t, p, mh, aliceEmail)
	bob := authflow.SignInViaMagicLink(ctx, t, p, mh, bobEmail)

	// Fetch user IDs for ref namespace construction.
	aliceID := getMe(ctx, t, p, alice.AccessToken).ID
	bobID := getMe(ctx, t, p, bob.AccessToken).ID

	// Alice creates an org and a session with default_mode=sync (all refs are
	// auto-merger eligible by default).
	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "AutoMerge Org")
	sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "AutoMerge Session")

	// Invite Bob to org and session.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, orgInviteID, orgInviteToken)

	sessionInviteID := inviteToSession(ctx, t, p, alice.AccessToken, orgID, sessionID, bobEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, bobEmail)
	acceptSessionInvite(ctx, t, p, bob.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// Subscribe to the session WebSocket before any pushes so we don't miss
	// events that arrive immediately after the push.
	aliceWS := wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)
	bobWS := wsclient.Connect(ctx, t, p.URL, sessionID, bob.AccessToken)

	// Alice clones the empty session repo and pushes a base commit to seed the
	// draft ref. The base ref (jam/<session>/base) is only allowed on the first
	// push to an empty repo; the portal creates draft pointing at the same commit
	// so the auto-merger has a starting point for subsequent sync-ref merges.
	aliceRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, aliceID, alice.AccessToken)
	baseRef := fmt.Sprintf("jam/%s/base", sessionID)
	_ = aliceRepo.Commit(ctx, t, "base.md", "session base content", "Alice: seed base")
	aliceRepo.Push(ctx, t, baseRef)

	// Alice's next commit must descend from the base commit so the auto-merger
	// finds a common ancestor with the draft tip. Alice's repo already has the
	// base commit as HEAD at this point (she just committed it), so the next
	// commit she makes will naturally be a child of the base commit.

	// Alice pushes a commit on her sync ref (alice.md only).
	aliceRef := fmt.Sprintf("jam/%s/%s/main", sessionID, aliceID)
	aliceSHA := aliceRepo.Commit(ctx, t, "alice.md", "Alice's contribution", "Alice: add alice.md")
	aliceRepo.Push(ctx, t, aliceRef)

	// Bob clones AFTER the base push so his clone has the base ref. He must
	// start his work from the base commit to share a common ancestor with the
	// draft tip. We fetch and reset to the base commit before adding bob.md.
	bobRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, bobID, bob.AccessToken)
	gitResetToRef(ctx, t, bobRepo.Dir, "origin/"+baseRef, bobID)

	bobRef := fmt.Sprintf("jam/%s/%s/main", sessionID, bobID)
	bobSHA := bobRepo.Commit(ctx, t, "bob.md", "Bob's contribution", "Bob: add bob.md")
	bobRepo.Push(ctx, t, bobRef)

	// Both Alice and Bob must receive merge.succeeded for Alice's commit.
	autoMergeTimeout := 20 * time.Second
	waitForMergeSucceeded(t, aliceWS, aliceSHA, autoMergeTimeout)
	waitForMergeSucceeded(t, bobWS, aliceSHA, autoMergeTimeout)

	// Both Alice and Bob must receive merge.succeeded for Bob's commit.
	waitForMergeSucceeded(t, aliceWS, bobSHA, autoMergeTimeout)
	waitForMergeSucceeded(t, bobWS, bobSHA, autoMergeTimeout)

	// Belt-and-suspenders: fetch and verify both source commits are reachable
	// from the draft ref in git log. This confirms the auto-merger actually
	// advanced draft (not just emitted an event).
	bobRepo.Fetch(ctx, t)
	draftRef := fmt.Sprintf("jam/%s/draft", sessionID)
	draftSHA := bobRepo.RevParse(ctx, t, draftRef)
	if draftSHA == "" {
		t.Fatal("draft ref is empty after auto-merge")
	}

	requireCommitInLog(t, bobRepo.Dir, draftSHA, aliceSHA,
		"Alice's commit must be reachable from draft after auto-merge")
	requireCommitInLog(t, bobRepo.Dir, draftSHA, bobSHA,
		"Bob's commit must be reachable from draft after auto-merge")
}

// waitForMergeSucceeded drains ws until a merge.succeeded event whose
// SourceCommit field starts with (or equals) wantSHA arrives, or timeout. The
// test is failed if the deadline expires first.
//
// The portal stores full 40-char SHAs; gitclient.Commit also returns a full
// 40-char SHA. Both sides are trimmed but kept in full for comparison.
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
			}
			if ev.Type != "merge.succeeded" {
				continue
			}
			var p mergeSucceededPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				t.Logf("waitForMergeSucceeded(%s): ignoring unparseable payload: %v", shortSHA, err)
				continue
			}
			// Accept either exact match or prefix (handles both full and short SHAs).
			if strings.HasPrefix(p.SourceSha, wantSHA) ||
				strings.HasPrefix(wantSHA, p.SourceSha) {
				return
			}
		case <-deadline:
			t.Fatalf("waitForMergeSucceeded(%s): timed out after %s waiting for merge.succeeded event",
				shortSHA, timeout)
		}
	}
}

// gitResetToRef runs `git fetch origin && git reset --hard <ref>` in repoDir,
// then re-configures git identity. Used to start a cloned working tree from a
// known commit (e.g. the session's base commit) before adding new work on top.
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

// requireCommitInLog asserts that commitSHA appears in the commit history
// reachable from tipSHA. It uses `git merge-base --is-ancestor` which returns
// exit 0 if commitSHA is an ancestor-of-or-equal-to tipSHA. This is an
// end-user observable outcome: anyone who clones and checks git-log can confirm
// the contribution is present.
func requireCommitInLog(t *testing.T, repoDir, tipSHA, commitSHA, msg string) {
	t.Helper()
	// git merge-base --is-ancestor exits 0 when commitSHA is an ancestor of
	// (or equal to) tipSHA; exits 1 when it is not.
	cmd := exec.Command("git", "merge-base", "--is-ancestor", commitSHA, tipSHA)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		// Exit 1 means "not an ancestor". Any other error is unexpected.
		t.Fatalf("requireCommitInLog: %s is not reachable from %s: %v\n--- %s",
			commitSHA[:7], tipSHA[:7], err, msg)
	}
}
