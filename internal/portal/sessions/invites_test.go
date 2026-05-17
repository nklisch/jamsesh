package sessions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
)

// ---------------------------------------------------------------------------
// Helpers for invite tests
// ---------------------------------------------------------------------------

func seedSessionMember(t *testing.T, s store.Store, orgID, sessionID, accountID, role string) {
	t.Helper()
	if err := s.AddSessionMember(context.Background(), store.AddSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: accountID,
		Role:      role,
		JoinedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed session member: %v", err)
	}
}

func seedSession(t *testing.T, s store.Store, orgID string) store.Session {
	t.Helper()
	sess, err := s.CreateSession(context.Background(), store.CreateSessionParams{
		ID:            uuid.New().String(),
		OrgID:         orgID,
		Name:          "Test Session",
		Goal:          "Test goal",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return sess
}

func rawTokenToHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/invites tests
// ---------------------------------------------------------------------------

func TestInviteToSession_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	_ = invitee // used implicitly via email below
	org := seedOrg(t, env.s, "Org", "org-invite")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	token := env.bearerToken(t, creator.ID)
	body := map[string]any{"email": "invitee@example.com"}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites",
		token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var invite openapi.Invite
	decodeBody(t, resp, &invite)
	if string(invite.InviteeEmail) != "invitee@example.com" {
		t.Errorf("unexpected invitee email: %q", invite.InviteeEmail)
	}
	if invite.SessionId != sess.ID {
		t.Errorf("unexpected session_id: %q", invite.SessionId)
	}
}

func TestInviteToSession_NotOrgMember_Returns403(t *testing.T) {
	env := newTestEnv(t)
	outsider := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "Org", "org-invite-403")
	sess := seedSession(t, env.s, org.ID)

	token := env.bearerToken(t, outsider.ID)
	body := map[string]any{"email": "target@example.com"}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites",
		token, body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestInviteToSession_NotSessionMember_Returns403(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	orgMember := seedAccount(t, env.s, "orgmember@example.com")
	org := seedOrg(t, env.s, "Org", "org-invite-session-403")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, orgMember.ID, "member")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")
	// orgMember is NOT a session member

	token := env.bearerToken(t, orgMember.ID)
	body := map[string]any{"email": "target@example.com"}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites",
		token, body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestInviteToSession_SessionNotFound_Returns404(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Org", "org-invite-404")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")

	token := env.bearerToken(t, creator.ID)
	body := map[string]any{"email": "target@example.com"}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/nonexistent/invites",
		token, body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept tests
// ---------------------------------------------------------------------------

func insertRawInvite(t *testing.T, s store.Store, orgID, sessionID, inviterID, inviteeEmail, rawToken string, expiresAt time.Time) store.SessionInvite {
	t.Helper()
	hash := rawTokenToHash(rawToken)
	invite, err := s.InsertSessionInvite(context.Background(), store.InsertSessionInviteParams{
		ID:               uuid.New().String(),
		OrgID:            orgID,
		SessionID:        sessionID,
		InviterAccountID: inviterID,
		InviteeEmail:     inviteeEmail,
		TokenHash:        hash,
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		t.Fatalf("insert invite: %v", err)
	}
	return invite
}

func TestAcceptSessionInvite_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	org := seedOrg(t, env.s, "Org", "org-accept")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	rawToken := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
		time.Now().UTC().Add(7*24*time.Hour))

	inviteeToken := env.bearerToken(t, invitee.ID)
	body := map[string]any{"token": rawToken}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
		inviteeToken, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var session openapi.Session
	decodeBody(t, resp, &session)

	// Invitee should now be a member.
	found := false
	for _, m := range session.Members {
		if m.AccountId == invitee.ID {
			found = true
			if m.Role != "member" {
				t.Errorf("expected role=member, got %q", m.Role)
			}
		}
	}
	if !found {
		t.Error("invitee not found in session members after accept")
	}
}

func TestAcceptSessionInvite_WrongToken_Returns401(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	org := seedOrg(t, env.s, "Org", "org-accept-bad-token")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	rawToken := "correcttokencorrecttokencorrecttokencorrecttokencorrecttokenco"
	invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
		time.Now().UTC().Add(7*24*time.Hour))

	inviteeToken := env.bearerToken(t, invitee.ID)
	body := map[string]any{"token": "wrongtokenwrongtokenwrongtokenwrongtokenwrongtokenwrongtokenwro"}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
		inviteeToken, body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAcceptSessionInvite_ExpiredToken_Returns401(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	org := seedOrg(t, env.s, "Org", "org-accept-expired")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	rawToken := "expiredtokenexpiredtokenexpiredtokenexpiredtokenexpiredtokenex"
	invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
		time.Now().UTC().Add(-1*time.Hour)) // expired

	inviteeToken := env.bearerToken(t, invitee.ID)
	body := map[string]any{"token": rawToken}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
		inviteeToken, body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAcceptSessionInvite_WrongEmail_Returns403(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	stranger := seedAccount(t, env.s, "stranger@example.com")
	org := seedOrg(t, env.s, "Org", "org-accept-wrong-email")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	rawToken := "correcttokencorrecttokencorrecttokencorrect1tokencorrect2to3en"
	invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
		time.Now().UTC().Add(7*24*time.Hour))

	// Stranger tries to accept an invite meant for invitee.
	strangerToken := env.bearerToken(t, stranger.ID)
	body := map[string]any{"token": rawToken}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
		strangerToken, body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAcceptSessionInvite_AlreadyAccepted_Returns409(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	org := seedOrg(t, env.s, "Org", "org-accept-dup")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	rawToken := "duptoken1234duptoken1234duptoken1234duptoken1234duptoken1234du"
	invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
		time.Now().UTC().Add(7*24*time.Hour))

	// Mark already accepted at the store level.
	now := time.Now().UTC()
	if err := env.s.MarkSessionInviteAccepted(context.Background(), store.MarkSessionInviteAcceptedParams{
		ID:                  invite.ID,
		AcceptedAt:          now,
		AcceptedByAccountID: invitee.ID,
	}); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}

	inviteeToken := env.bearerToken(t, invitee.ID)
	body := map[string]any{"token": rawToken}

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
		inviteeToken, body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove tests
// ---------------------------------------------------------------------------

func TestRemoveSessionMember_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	member := seedAccount(t, env.s, "member@example.com")
	org := seedOrg(t, env.s, "Org", "org-remove")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, member.ID, "member")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")
	seedSessionMember(t, env.s, org.ID, sess.ID, member.ID, "member")

	token := env.bearerToken(t, creator.ID)

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/members/"+member.ID+"/remove",
		token, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// Verify the member was removed.
	_, err := env.s.GetSessionMember(context.Background(), store.GetSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sess.ID,
		AccountID: member.ID,
	})
	if err == nil {
		t.Error("expected member to be removed, but GetSessionMember returned no error")
	}
}

func TestRemoveSessionMember_NotCreator_Returns403(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	member1 := seedAccount(t, env.s, "member1@example.com")
	member2 := seedAccount(t, env.s, "member2@example.com")
	org := seedOrg(t, env.s, "Org", "org-remove-403")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, member1.ID, "member")
	seedOrgMember(t, env.s, org.ID, member2.ID, "member")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")
	seedSessionMember(t, env.s, org.ID, sess.ID, member1.ID, "member")
	seedSessionMember(t, env.s, org.ID, sess.ID, member2.ID, "member")

	// member1 tries to remove member2 — only the creator can do this.
	token := env.bearerToken(t, member1.ID)

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/members/"+member2.ID+"/remove",
		token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRemoveSessionMember_SessionNotFound_Returns404(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Org", "org-remove-session-404")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")

	token := env.bearerToken(t, creator.ID)

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/nonexistent/members/someone/remove",
		token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRemoveSessionMember_TargetNotMember_Returns404(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	nonMember := seedAccount(t, env.s, "nonmember@example.com")
	org := seedOrg(t, env.s, "Org", "org-remove-target-404")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	sess := seedSession(t, env.s, org.ID)
	seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

	token := env.bearerToken(t, creator.ID)

	resp := postJSON(t, env.srv,
		"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/members/"+nonMember.ID+"/remove",
		token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
