package accounts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	portalauth "jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/accounts"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// captureSenderOrgs captures Send calls for asserting invite email delivery.
type captureSenderOrgs struct {
	mu        sync.Mutex
	recipient string
	subject   string
	body      string
	calls     int
}

func (c *captureSenderOrgs) Send(_ context.Context, recipient, subject, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recipient = recipient
	c.subject = subject
	c.body = body
	c.calls++
	return nil
}

func (c *captureSenderOrgs) lastRecipient() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recipient
}

func (c *captureSenderOrgs) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// orgsMembersTestEnv wires the 3 new endpoints plus Bearer + RequireOrgRole middleware.
type orgsMembersTestEnv struct {
	srv    *httptest.Server
	svc    tokens.Service
	s      store.Store
	sender *captureSenderOrgs
}

func newOrgsMembersTestEnv(t *testing.T) *orgsMembersTestEnv {
	t.Helper()
	s := openStore(t)
	svc := tokens.New(s)
	sender := &captureSenderOrgs{}
	h := accounts.New(s, sender, "https://portal.example.com")
	strictHandler := openapi.NewStrictHandler(&accountsOnlyStrict{h}, nil)

	// Build an api wrapper for path-param routes.
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler: strictHandler,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))

		// Org members: creator or member role required.
		r.Group(func(r chi.Router) {
			r.Use(portalauth.RequireOrgRole(s, "creator", "member"))
			r.Get("/api/orgs/{orgID}/members", apiWrapper.ListOrgMembers)
		})

		// Create invite: creator role required.
		r.Group(func(r chi.Router) {
			r.Use(portalauth.RequireOrgRole(s, "creator"))
			r.Post("/api/orgs/{orgID}/invites", apiWrapper.CreateOrgInvite)
		})

		// Accept invite: Bearer only.
		r.Post("/api/orgs/{orgID}/invites/{inviteID}/accept", apiWrapper.AcceptOrgInvite)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &orgsMembersTestEnv{srv: srv, svc: svc, s: s, sender: sender}
}

func (e *orgsMembersTestEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

// hashToken returns the SHA-256 hex hash of a raw token string.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// seedInvite inserts an org_invite row directly into the store.
func seedInvite(t *testing.T, s store.Store, orgID, inviterID, email, rawToken string, expiresAt time.Time) store.OrgInvite {
	t.Helper()
	inv, err := s.InsertOrgInvite(context.Background(), store.InsertOrgInviteParams{
		ID:               uuid.New().String(),
		OrgID:            orgID,
		InviterAccountID: inviterID,
		RecipientEmail:   email,
		TokenHash:        hashToken(rawToken),
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	return inv
}

// ---------------------------------------------------------------------------
// GET /api/orgs/{orgID}/members
// ---------------------------------------------------------------------------

func TestListOrgMembers_HappyPath(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator@example.com")
	member := seedAccount(t, env.s, "member@example.com")
	org := seedOrg(t, env.s, "TestOrg", "testorg")
	seedMember(t, env.s, org.ID, creator.ID, "creator")
	seedMember(t, env.s, org.ID, member.ID, "member")

	tok := env.bearerToken(t, creator.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID+"/members", tok)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(body) != 2 {
		t.Fatalf("expected 2 members, got %d", len(body))
	}
}

func TestListOrgMembers_NotMember_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	outsider := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "SecretOrg", "secretorg")

	tok := env.bearerToken(t, outsider.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID+"/members", tok)

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/invites
// ---------------------------------------------------------------------------

func TestCreateOrgInvite_HappyPath(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "boss@example.com")
	org := seedOrg(t, env.s, "MyOrg", "myorg")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	tok := env.bearerToken(t, creator.ID)
	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/invites", tok,
		map[string]any{"email": "invitee@example.com"})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["id"] == nil || body["id"] == "" {
		t.Error("expected non-empty invite id")
	}
	if body["recipient_email"] != "invitee@example.com" {
		t.Errorf("recipient_email: got %v", body["recipient_email"])
	}
	if body["expires_at"] == nil {
		t.Error("expected expires_at")
	}

	// Verify sender was called.
	if env.sender.callCount() != 1 {
		t.Errorf("expected 1 email sent, got %d", env.sender.callCount())
	}
	if env.sender.lastRecipient() != "invitee@example.com" {
		t.Errorf("email recipient: got %q", env.sender.lastRecipient())
	}
}

func TestCreateOrgInvite_NonCreator_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	acc := seedAccount(t, env.s, "regularuser@example.com")
	org := seedOrg(t, env.s, "Org", "org")
	seedMember(t, env.s, org.ID, acc.ID, "member") // not creator

	tok := env.bearerToken(t, acc.ID)
	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/invites", tok,
		map[string]any{"email": "invitee@example.com"})

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/invites/{inviteID}/accept
// ---------------------------------------------------------------------------

func TestAcceptOrgInvite_HappyPath(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	inviter := seedAccount(t, env.s, "inviter@example.com")
	invitee := seedAccount(t, env.s, "invitee@example.com")
	org := seedOrg(t, env.s, "CoolOrg", "coolorg")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "deadbeef1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	inv := seedInvite(t, env.s, org.ID, inviter.ID, invitee.Email, rawToken, time.Now().Add(7*24*time.Hour))

	tok := env.bearerToken(t, invitee.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": rawToken})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["id"] != org.ID {
		t.Errorf("org id: got %v, want %s", body["id"], org.ID)
	}

	// Verify the invitee is now a member.
	m, err := env.s.GetOrgMember(context.Background(), store.GetOrgMemberParams{
		OrgID:     org.ID,
		AccountID: invitee.ID,
	})
	if err != nil {
		t.Fatalf("get org member after accept: %v", err)
	}
	if m.Role != "member" {
		t.Errorf("expected role member, got %s", m.Role)
	}
}

func TestAcceptOrgInvite_WrongToken_Returns401(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	inviter := seedAccount(t, env.s, "inv2@example.com")
	invitee := seedAccount(t, env.s, "invitee2@example.com")
	org := seedOrg(t, env.s, "Org2", "org2")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "correcttoken1234567890abcdef1234567890abcdef1234567890abcdef1234"
	inv := seedInvite(t, env.s, org.ID, inviter.ID, invitee.Email, rawToken, time.Now().Add(7*24*time.Hour))

	tok := env.bearerToken(t, invitee.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": "wrongtoken"})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAcceptOrgInvite_ExpiredToken_Returns401(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	inviter := seedAccount(t, env.s, "inv3@example.com")
	invitee := seedAccount(t, env.s, "invitee3@example.com")
	org := seedOrg(t, env.s, "Org3", "org3")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "expiredtoken234567890abcdef1234567890abcdef1234567890abcdef12345"
	inv := seedInvite(t, env.s, org.ID, inviter.ID, invitee.Email, rawToken, time.Now().Add(-1*time.Hour)) // expired

	tok := env.bearerToken(t, invitee.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": rawToken})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAcceptOrgInvite_AlreadyAccepted_Returns409(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	inviter := seedAccount(t, env.s, "inv4@example.com")
	invitee := seedAccount(t, env.s, "invitee4@example.com")
	org := seedOrg(t, env.s, "Org4", "org4")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "alreadyacceptedtoken90abcdef1234567890abcdef1234567890abcdef12345"
	inv := seedInvite(t, env.s, org.ID, inviter.ID, invitee.Email, rawToken, time.Now().Add(7*24*time.Hour))

	// Mark it accepted directly.
	now := time.Now().UTC()
	if err := env.s.MarkOrgInviteAccepted(context.Background(), store.MarkOrgInviteAcceptedParams{
		ID:                  inv.ID,
		AcceptedAt:          now,
		AcceptedByAccountID: invitee.ID,
	}); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}

	tok := env.bearerToken(t, invitee.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": rawToken})

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestAcceptOrgInvite_WrongRecipientEmail_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	inviter := seedAccount(t, env.s, "inv5@example.com")
	intendedRecipient := seedAccount(t, env.s, "intended@example.com")
	wrongAccount := seedAccount(t, env.s, "wrong@example.com")
	org := seedOrg(t, env.s, "Org5", "org5")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "wrongrecipienttoken90abcdef1234567890abcdef1234567890abcdef12345"
	inv := seedInvite(t, env.s, org.ID, inviter.ID, intendedRecipient.Email, rawToken, time.Now().Add(7*24*time.Hour))

	// wrongAccount tries to accept an invite meant for intendedRecipient.
	tok := env.bearerToken(t, wrongAccount.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": rawToken})

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

// Ensure the import is used.
var _ = openapi_types.Email("")
