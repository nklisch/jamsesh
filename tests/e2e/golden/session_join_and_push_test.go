// Invariant: when two agents join the same session and push commits on
// independent refs, each agent's local working copy can `git fetch` the
// peer's ref tip, AND the portal's WebSocket event stream delivers a
// commit.arrived event for each push within 5 seconds.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

// sessionRef holds the data returned by POST /api/orgs/{orgID}/sessions.
type sessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// inviteRef is the minimal subset of the Invite schema we need.
type inviteRef struct {
	ID string `json:"id"`
}

// meResponse is the minimal /me response we need to extract a user ID.
type meResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func TestSessionLifecycleJoinAndPush(t *testing.T) {
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

	// Fetch user IDs from /me — needed for the git ref namespace.
	aliceID := getMe(ctx, t, p, alice.AccessToken).ID
	bobID := getMe(ctx, t, p, bob.AccessToken).ID

	// Alice creates an org.
	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Session Lifecycle Org")

	// Alice creates a session.
	sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "Lifecycle Test Session")

	// Alice invites Bob to the session. Bob must be an org member first for the
	// session invite to succeed (the handler checks org membership).
	// 1. Invite Bob to org and have him accept.
	orgInviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)
	// Capture Bob's org invite token BEFORE his magic-link token can overwrite it
	// in MailHog's latest-message slot.
	orgInviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, orgInviteID, orgInviteToken)

	// 2. Now invite Bob to the session.
	sessionInviteID := inviteToSession(ctx, t, p, alice.AccessToken, orgID, sessionID, bobEmail)
	sessionInviteToken := extractSessionInviteToken(ctx, t, mh, bobEmail)
	acceptSessionInvite(ctx, t, p, bob.AccessToken, orgID, sessionID, sessionInviteID, sessionInviteToken)

	// Both agents subscribe to the session WebSocket.
	aliceWS := wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)
	bobWS := wsclient.Connect(ctx, t, p.URL, sessionID, bob.AccessToken)

	// Alice clones the (empty) session repo and pushes a commit on her ref.
	aliceRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, aliceID, alice.AccessToken)
	aliceRef := "jam/" + sessionID + "/" + aliceID + "/main"
	aliceRepo.Commit(ctx, t, "alice.md", "Alice's work", "Alice: initial commit")
	aliceRepo.Push(ctx, t, aliceRef)

	// Both Alice and Bob must see commit.arrived within 5 seconds of Alice's push.
	aliceWS.WaitFor(t, "commit.arrived", 5*time.Second)
	bobWS.WaitFor(t, "commit.arrived", 5*time.Second)

	// Bob clones the session repo and pushes a commit on his own ref.
	bobRepo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, bobID, bob.AccessToken)
	bobRef := "jam/" + sessionID + "/" + bobID + "/main"
	bobSHA := bobRepo.Commit(ctx, t, "bob.md", "Bob's work", "Bob: initial commit")
	bobRepo.Push(ctx, t, bobRef)

	// Bob's WS stream sees his own commit.arrived too.
	bobWS.WaitFor(t, "commit.arrived", 5*time.Second)

	// Alice fetches from the session remote and can see Bob's ref tip.
	aliceRepo.Fetch(ctx, t)
	fetchedBobSHA := aliceRepo.RevParse(ctx, t, bobRef)
	if fetchedBobSHA != bobSHA {
		t.Fatalf("Alice's git fetch: expected Bob's SHA %s, got %s", bobSHA, fetchedBobSHA)
	}
}

// ---------------------------------------------------------------------------
// Session-scoped API helpers (not in authflow because they are session-
// specific; keep them here unless another spec also needs them).
// ---------------------------------------------------------------------------

// createSession calls POST /api/orgs/{orgID}/sessions and returns the new
// session's ID.
func createSession(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "E2E test session",
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
	var s sessionRef
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
	var inv inviteRef
	if err := json.Unmarshal(respBody, &inv); err != nil {
		t.Fatalf("inviteToSession: decode: %v\nbody: %s", err, respBody)
	}
	if inv.ID == "" {
		t.Fatalf("inviteToSession: empty id in response")
	}
	return inv.ID
}

// extractSessionInviteToken polls MailHog for a session invite email to the
// given recipient and returns the raw token. The session invite email uses the
// same token=<raw> URL pattern as the org invite, so the shared InviteTokenRE
// regex works.
func extractSessionInviteToken(ctx context.Context, t *testing.T, mh *mailhog.MailHog, email string) string {
	t.Helper()
	// Use a long enough poll timeout; email delivery is asynchronous.
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

// getMe calls GET /api/me and returns the caller's user record.
func getMe(ctx context.Context, t *testing.T, p *portal.Portal, accessToken string) meResponse {
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
	var me meResponse
	if err := json.Unmarshal(respBody, &me); err != nil {
		t.Fatalf("getMe: decode: %v\nbody: %s", err, respBody)
	}
	return me
}
