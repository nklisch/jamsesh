// Invariant: Agent A can fork via MCP from a draft commit, and post a comment
// addressed to Agent B. When Agent B's user-prompt-submit hook runs next, the
// additionalContext output contains the comment text.
package golden_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/binary"
	"jamsesh/tests/e2e/fixtures/ccdriver"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/mcpclient"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

func TestForkAndComment(t *testing.T) {
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

	binPath := binary.Build(t)

	agentAEmail := randEmail(t, "agent-a")
	agentBEmail := randEmail(t, "agent-b")

	// Sign in both agents.
	agentA := authflow.SignInViaMagicLink(ctx, t, p, mh, agentAEmail)
	agentB := authflow.SignInViaMagicLink(ctx, t, p, mh, agentBEmail)

	agentAID := getMe(ctx, t, p, agentA.AccessToken).ID
	agentBID := getMe(ctx, t, p, agentB.AccessToken).ID

	// Agent A creates the org and session.
	orgID := authflow.CreateOrg(ctx, t, p, agentA.AccessToken, "ForkComment Org")
	sessionID := createSession(ctx, t, p, agentA.AccessToken, orgID, "ForkComment Session")

	// Invite Agent B to org and session.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, agentA.AccessToken, orgID, agentBEmail)
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, agentBEmail)
	authflow.AcceptInvite(ctx, t, p, agentB.AccessToken, orgID, orgInviteID, orgInviteToken)

	sessionInviteID := inviteToSession(ctx, t, p, agentA.AccessToken, orgID, sessionID, agentBEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, agentBEmail)
	acceptSessionInvite(ctx, t, p, agentB.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// Subscribe to the session WebSocket (Agent B watches for ref.forked events).
	agentBWS := wsclient.Connect(ctx, t, p.URL, sessionID, agentB.AccessToken)

	// Agent A pushes a base commit to seed the draft ref (required before any
	// sync-mode pushes can be auto-merged), then pushes an initial commit on
	// their sync ref to establish a commit SHA suitable for forking.
	agentARepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, agentAID, agentA.AccessToken)
	baseRef := fmt.Sprintf("jam/%s/base", sessionID)
	_ = agentARepo.Commit(ctx, t, "base.md", "session base", "Agent A: seed base")
	agentARepo.Push(ctx, t, baseRef)

	// Agent A's next commit descends from the base commit (HEAD was just set
	// to the base commit above), so the auto-merger finds a common ancestor.
	agentARef := fmt.Sprintf("jam/%s/%s/main", sessionID, agentAID)
	commitSHA := agentARepo.Commit(ctx, t, "init.md", "initial content", "Agent A: initial commit")
	agentARepo.Push(ctx, t, agentARef)

	// Wait for the commit to arrive so the portal's repo has it before we fork.
	agentBWS.WaitFor(t, "commit.arrived", 10*time.Second)

	// ---------------------------------------------------------------------------
	// Fork via MCP (Agent A calls the fork tool)
	// ---------------------------------------------------------------------------

	mcpA := mcpclient.New(t, p.URL, agentA.AccessToken)

	forkResult, err := mcpA.Fork(ctx, mcpclient.ForkArgs{
		SessionID:       sessionID,
		TargetCommitSHA: commitSHA,
	})
	if err != nil {
		t.Fatalf("Agent A fork MCP call: %v", err)
	}
	if forkResult.Ref == "" {
		t.Fatal("fork MCP result: empty ref")
	}
	if !strings.Contains(forkResult.Ref, "fork-") {
		t.Errorf("fork MCP result: expected ref to contain 'fork-', got %q", forkResult.Ref)
	}

	// Both agents see the ref.forked WebSocket event.
	agentBWS.WaitFor(t, "ref.forked", 10*time.Second)

	// ---------------------------------------------------------------------------
	// Post a comment addressed to Agent B (Agent A calls the post_comment tool)
	// ---------------------------------------------------------------------------

	const commentText = "Agent B, please review this initial commit."
	addressedTo := "@" + agentBID // addressed by account ID

	postResult, err := mcpA.PostComment(ctx, mcpclient.PostCommentArgs{
		SessionID:   sessionID,
		CommitSHA:   commitSHA,
		Body:        commentText,
		AddressedTo: &addressedTo,
	})
	if err != nil {
		t.Fatalf("Agent A post_comment MCP call: %v", err)
	}
	if postResult.CommentID == "" {
		t.Fatal("post_comment MCP result: empty comment_id")
	}

	// ---------------------------------------------------------------------------
	// Seed Agent B's plugin state directory and invoke user-prompt-submit
	// ---------------------------------------------------------------------------

	// The user-prompt-submit hook reads:
	//   $CLAUDE_PLUGIN_DATA/token                     – bearer token
	//   $CLAUDE_PLUGIN_DATA/sessions/<sid>/org_id     – org ID
	//   $CLAUDE_PLUGIN_DATA/sessions/<sid>/ref        – agent's ref
	//   $CLAUDE_PLUGIN_DATA/sessions/<sid>/account_id – account ID
	//   $CLAUDE_PLUGIN_DATA/sessions/<sid>/instance_id – CC instance ID
	//   $CLAUDE_PLUGIN_DATA/sessions/<sid>/last_seen_seq – event cursor
	// The portal URL is resolved from JAMSESH_PORTAL_URL env var (highest priority).

	dataDir := t.TempDir()
	agentBRef := fmt.Sprintf("jam/%s/%s/main", sessionID, agentBID)
	seedPluginState(t, dataDir, agentB.AccessToken, sessionID, orgID, agentBRef, agentBID)

	d := &ccdriver.Driver{
		BinaryPath: binPath,
		DataDir:    dataDir,
		ExtraEnv:   []string{"JAMSESH_PORTAL_URL=" + p.URL},
	}

	out, err := d.SubmitPrompt(ctx, ccdriver.UserPromptSubmitInput{
		SessionID:      sessionID,
		TranscriptPath: filepath.Join(t.TempDir(), "transcript.json"),
		Cwd:            t.TempDir(),
	})
	if err != nil {
		t.Fatalf("user-prompt-submit hook: %v", err)
	}

	// The additionalContext from the digest must mention the comment text
	// because the comment is addressed to Agent B and is unresolved.
	if !strings.Contains(out.AdditionalContext, commentText) {
		t.Errorf("user-prompt-submit additionalContext does not contain the comment text %q\nadditionalContext:\n%s",
			commentText, out.AdditionalContext)
	}
}

// seedPluginState writes the per-session state files that the jamsesh binary
// needs to function as Agent B when invoked via ccdriver.
//
// Layout under dataDir/:
//
//	token                              – bearer access token
//	sessions/<sessionID>/org_id        – org that owns the session
//	sessions/<sessionID>/ref           – agent's git ref
//	sessions/<sessionID>/account_id    – agent's account ID
//	sessions/<sessionID>/instance_id   – CC instance ID (arbitrary; single session)
//	sessions/<sessionID>/last_seen_seq – event cursor (start at 0)
func seedPluginState(t *testing.T, dataDir, token, sessionID, orgID, ref, accountID string) {
	t.Helper()
	writeFile := func(rel, value string) {
		t.Helper()
		full := filepath.Join(dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("seedPluginState: mkdirall %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(value), 0o600); err != nil {
			t.Fatalf("seedPluginState: write %s: %v", rel, err)
		}
	}

	writeFile("token", token)
	sessBase := filepath.Join("sessions", sessionID)
	writeFile(filepath.Join(sessBase, "org_id"), orgID)
	writeFile(filepath.Join(sessBase, "ref"), ref)
	writeFile(filepath.Join(sessBase, "account_id"), accountID)
	writeFile(filepath.Join(sessBase, "instance_id"), "e2e-test-instance")
	writeFile(filepath.Join(sessBase, "last_seen_seq"), "0")
}
