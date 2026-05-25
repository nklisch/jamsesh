package sessions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/db/store/storetest"
	"jamsesh/internal/portal/senders"
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
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestInviteToSession_NotOrgMember_Returns403(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestInviteToSession_NotSessionMember_Returns403(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestInviteToSession_SessionNotFound_Returns404(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

// TestInviteToSession_SenderError_Returns503DepSMTPUnavailable verifies
// the session-invite path wraps Sender failures into the typed dep
// envelope (HTTP 503, error=dep.smtp_unavailable, Retry-After:5).
func TestInviteToSession_SenderError_Returns503DepSMTPUnavailable(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator-fail@example.com")
			org := seedOrg(t, env.s, "FailOrg", "fail-org-invite")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			// Inject a transient sender failure. The handler must wrap with
			// deperr.WrapSMTP so the translator surfaces the typed envelope.
			env.sender.err = fmt.Errorf("%w: forced", senders.ErrTransient)

			token := env.bearerToken(t, creator.ID)
			body := map[string]any{"email": "invitee@example.com"}

			resp := postJSON(t, env.srv,
				"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites",
				token, body)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("want 503, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type: want application/json; charset=utf-8, got %q", ct)
			}
			if ra := resp.Header.Get("Retry-After"); ra != "5" {
				t.Errorf("Retry-After: want 5, got %q", ra)
			}
			var env2 map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&env2); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if code, _ := env2["error"].(string); code != "dep.smtp_unavailable" {
				t.Errorf("error code: want dep.smtp_unavailable, got %q", code)
			}
		})
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
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-accept")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			// Invitee must be an org member; default policy is members_only.
			seedOrgMember(t, env.s, org.ID, invitee.ID, "member")
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
		})
	}
}

func TestAcceptSessionInvite_WrongToken_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestAcceptSessionInvite_ExpiredToken_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestAcceptSessionInvite_WrongEmail_Returns403(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestAcceptSessionInvite_AlreadyAccepted_Returns409(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

// ---------------------------------------------------------------------------
// AcceptSessionInvite — per-org session_invite_policy cross-product tests
// ---------------------------------------------------------------------------

// TestAcceptSessionInvite_MembersOnlyPolicy_NonMember verifies that a
// non-org-member is rejected with 403 auth.org_membership_required when the
// org's session_invite_policy is "members_only" (the default).
func TestAcceptSessionInvite_MembersOnlyPolicy_NonMember(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee-nonmember@example.com")
			org := seedOrg(t, env.s, "Org", "pol-mo-nonmember")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			// Invitee is NOT added to org_members — policy should block them.
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "polmononmemberpolmononmemberpolmononmemberpolmononmemberpolmo"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			body := map[string]any{"token": rawToken}

			resp := postJSON(t, env.srv,
				"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
				inviteeToken, body)
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("members_only + non-member: expected 403, got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "auth.org_membership_required" {
				t.Errorf("error code: want auth.org_membership_required, got %q", errEnv.Error)
			}
		})
	}
}

// TestAcceptSessionInvite_MembersOnlyPolicy_Member verifies that an existing
// org member can accept an invite when the org's policy is "members_only".
// This covers the happy-path under the restrictive policy explicitly.
func TestAcceptSessionInvite_MembersOnlyPolicy_Member(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee-member@example.com")
			org := seedOrg(t, env.s, "Org", "pol-mo-member")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			seedOrgMember(t, env.s, org.ID, invitee.ID, "member") // is an org member
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "polmomemberpolmomemberpolmomemberpolmomemberpolmomemberpolmome"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			body := map[string]any{"token": rawToken}

			resp := postJSON(t, env.srv,
				"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
				inviteeToken, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("members_only + org member: expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestAcceptSessionInvite_OpenPolicy_NonMember verifies that a non-org-member
// can accept an invite when the org's policy is flipped to "open".
func TestAcceptSessionInvite_OpenPolicy_NonMember(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee-open-nonmember@example.com")
			org := seedOrg(t, env.s, "Org", "pol-open-nonmember")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			// Invitee is NOT an org member — open policy should allow them anyway.
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			// Flip policy to "open".
			if err := env.s.UpdateOrgSessionInvitePolicy(context.Background(), store.UpdateOrgSessionInvitePolicyParams{
				ID:                  org.ID,
				SessionInvitePolicy: "open",
			}); err != nil {
				t.Fatalf("update org policy: %v", err)
			}

			rawToken := "polopennonmemberpolopennonmemberpolopennonmemberpolopennonmember"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			body := map[string]any{"token": rawToken}

			resp := postJSON(t, env.srv,
				"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
				inviteeToken, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("open policy + non-member: expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestAcceptSessionInvite_OpenPolicy_Member verifies that an org member can
// also accept an invite under the "open" policy (belt-and-suspenders).
func TestAcceptSessionInvite_OpenPolicy_Member(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee-open-member@example.com")
			org := seedOrg(t, env.s, "Org", "pol-open-member")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			seedOrgMember(t, env.s, org.ID, invitee.ID, "member") // is an org member
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			// Flip policy to "open".
			if err := env.s.UpdateOrgSessionInvitePolicy(context.Background(), store.UpdateOrgSessionInvitePolicyParams{
				ID:                  org.ID,
				SessionInvitePolicy: "open",
			}); err != nil {
				t.Fatalf("update org policy: %v", err)
			}

			rawToken := "polopenmemberpolopenmemberpolopenmemberpolopenmemberpolopenmember"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			body := map[string]any{"token": rawToken}

			resp := postJSON(t, env.srv,
				"/api/orgs/"+org.ID+"/sessions/"+sess.ID+"/invites/"+invite.ID+"/accept",
				inviteeToken, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("open policy + org member: expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID} tests
// ---------------------------------------------------------------------------

func inviteDetailsPath(orgID, sessID, inviteID, rawToken string) string {
	path := "/api/orgs/" + orgID + "/sessions/" + sessID + "/invites/" + inviteID
	if rawToken != "" {
		path += "?token=" + rawToken
	}
	return path
}

func TestGetSessionInvite_HappyPath(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "My Org", "org-get-invite")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "happypathtokenhappypathtokenhappypathtokenhappypathtokenhappypa"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, rawToken), inviteeToken)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}

			var details openapi.SessionInviteDetails
			decodeBody(t, resp, &details)

			if details.InviteId != invite.ID {
				t.Errorf("invite_id: want %q, got %q", invite.ID, details.InviteId)
			}
			if details.OrgName != org.Name {
				t.Errorf("org_name: want %q, got %q", org.Name, details.OrgName)
			}
			if details.SessionId != sess.ID {
				t.Errorf("session_id: want %q, got %q", sess.ID, details.SessionId)
			}
			if details.SessionName != sess.Name {
				t.Errorf("session_name: want %q, got %q", sess.Name, details.SessionName)
			}
			if details.InvitedByName != creator.DisplayName {
				t.Errorf("invited_by_name: want %q, got %q", creator.DisplayName, details.InvitedByName)
			}
			if string(details.YourRoleOnAccept) != "member" {
				t.Errorf("your_role_on_accept: want %q, got %q", "member", details.YourRoleOnAccept)
			}
		})
	}
}

func TestGetSessionInvite_InvalidToken_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-bad-token")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "correcttokencorrecttokencorrecttokencorrecttokencorrecttokencor"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			wrongToken := "wrongtokenwrongtokenwrongtokenwrongtokenwrongtokenwrongtokenwron"
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, wrongToken), inviteeToken)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 for wrong token, got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "auth.invalid_token" {
				t.Errorf("error code: want auth.invalid_token, got %q", errEnv.Error)
			}
		})
	}
}

func TestGetSessionInvite_MissingToken_Returns400(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			// oapi-codegen validates required query params before the handler is
			// reached; missing ?token= yields a 400 Bad Request from the framework.
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-missing-token")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "anytokenanytokenanytokenanytokenanytokenanytokenanytokenanytoken"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			// inviteDetailsPath with empty token omits the ?token= param entirely.
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, ""), inviteeToken)
			// oapi-codegen rejects missing required query param before handler runs.
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for missing token, got %d", resp.StatusCode)
			}
		})
	}
}

func TestGetSessionInvite_UnknownInvite_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			// Unknown invite-id must return 401 (not 404) to prevent invite-id enumeration.
			env := newTestEnv(t, h.Open(t))
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-unknown")
			sess := seedSession(t, env.s, org.ID)

			inviteeToken := env.bearerToken(t, invitee.ID)
			rawToken := "sometokensometokensometokensometokensometokensometokensometokenso"
			resp := getRequest(t, env.srv,
				inviteDetailsPath(org.ID, sess.ID, "nonexistent-invite-id", rawToken),
				inviteeToken)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 for unknown invite id, got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "auth.invalid_token" {
				t.Errorf("error code: want auth.invalid_token, got %q", errEnv.Error)
			}
		})
	}
}

func TestGetSessionInvite_WrongEmail_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			// On GET, email mismatch returns 401 (not 403) to avoid leaking that the
			// invite exists for a different email address.
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			stranger := seedAccount(t, env.s, "stranger@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-wrong-email")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "wrongemailtokenwrongemailtokenwrongemailtokenwrongemailtokenw"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			// Stranger tries to read an invite meant for invitee.
			strangerToken := env.bearerToken(t, stranger.ID)
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, rawToken), strangerToken)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 for wrong email (not 403), got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "auth.invalid_token" {
				t.Errorf("error code: want auth.invalid_token, got %q", errEnv.Error)
			}
		})
	}
}

func TestGetSessionInvite_Expired_Returns401(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-expired")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "expiredtokenexpiredtokenexpiredtokenexpiredtokenexpiredtokenex"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(-1*time.Hour)) // already expired

			inviteeToken := env.bearerToken(t, invitee.ID)
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, rawToken), inviteeToken)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 for expired invite, got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "auth.invalid_token" {
				t.Errorf("error code: want auth.invalid_token, got %q", errEnv.Error)
			}
			if errEnv.Message != "invite expired" {
				t.Errorf("message: want %q, got %q", "invite expired", errEnv.Message)
			}
		})
	}
}

func TestGetSessionInvite_AlreadyAccepted_Returns409(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-accepted")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "acceptedtokenacceptedtokenacceptedtokenacceptedtokenacceptedto"
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
			resp := getRequest(t, env.srv, inviteDetailsPath(org.ID, sess.ID, invite.ID, rawToken), inviteeToken)
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("expected 409 for already-accepted invite, got %d", resp.StatusCode)
			}

			var errEnv openapi.ErrorEnvelope
			decodeBody(t, resp, &errEnv)
			if errEnv.Error != "invite.already_accepted" {
				t.Errorf("error code: want invite.already_accepted, got %q", errEnv.Error)
			}
		})
	}
}

func TestGetSessionInvite_NoMutation(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			// GET must not mutate state: calling GET twice should succeed both times,
			// and then POST accept should still work (the invite was not consumed by GET).
			env := newTestEnv(t, h.Open(t))
			creator := seedAccount(t, env.s, "creator@example.com")
			invitee := seedAccount(t, env.s, "invitee@example.com")
			org := seedOrg(t, env.s, "Org", "org-get-invite-nomut")
			seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
			// Invitee must be an org member so POST accept succeeds under members_only policy.
			seedOrgMember(t, env.s, org.ID, invitee.ID, "member")
			sess := seedSession(t, env.s, org.ID)
			seedSessionMember(t, env.s, org.ID, sess.ID, creator.ID, "creator")

			rawToken := "nomuttokennomuttokennomuttokennomuttokennomuttokennomuttokennomut"
			invite := insertRawInvite(t, env.s, org.ID, sess.ID, creator.ID, invitee.Email, rawToken,
				time.Now().UTC().Add(7*24*time.Hour))

			inviteeToken := env.bearerToken(t, invitee.ID)
			detailsPath := inviteDetailsPath(org.ID, sess.ID, invite.ID, rawToken)

			// First GET — must return 200.
			resp1 := getRequest(t, env.srv, detailsPath, inviteeToken)
			if resp1.StatusCode != http.StatusOK {
				t.Fatalf("first GET: expected 200, got %d", resp1.StatusCode)
			}

			// Second GET — must still return 200 (no state was consumed).
			resp2 := getRequest(t, env.srv, detailsPath, inviteeToken)
			if resp2.StatusCode != http.StatusOK {
				t.Fatalf("second GET: expected 200, got %d", resp2.StatusCode)
			}

			// POST accept — must still work (invite not consumed by GET).
			acceptPath := "/api/orgs/" + org.ID + "/sessions/" + sess.ID + "/invites/" + invite.ID + "/accept"
			resp3 := postJSON(t, env.srv, acceptPath, inviteeToken, map[string]any{"token": rawToken})
			if resp3.StatusCode != http.StatusOK {
				t.Fatalf("POST accept after GET: expected 200, got %d", resp3.StatusCode)
			}

			// GET after accept — must return 409 (already accepted).
			resp4 := getRequest(t, env.srv, detailsPath, inviteeToken)
			if resp4.StatusCode != http.StatusConflict {
				t.Fatalf("GET after accept: expected 409, got %d", resp4.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove tests
// ---------------------------------------------------------------------------

func TestRemoveSessionMember_HappyPath(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestRemoveSessionMember_NotCreator_Returns403(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestRemoveSessionMember_SessionNotFound_Returns404(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}

func TestRemoveSessionMember_TargetNotMember_Returns404(t *testing.T) {
	for _, h := range storetest.Stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t))
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
		})
	}
}
