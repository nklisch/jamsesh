package accounts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"jamsesh/internal/portal/accounts"
	portalauth "jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// captureSenderOrgs captures Send calls for asserting invite email delivery.
// Set err to inject a failure for dep-failure-envelope tests.
type captureSenderOrgs struct {
	mu        sync.Mutex
	recipient string
	subject   string
	body      string
	calls     int
	err       error
}

func (c *captureSenderOrgs) Send(_ context.Context, recipient, subject, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recipient = recipient
	c.subject = subject
	c.body = body
	c.calls++
	return c.err
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
	// Wire the dep-failure translator so sender errors surface as the
	// typed dep.smtp_unavailable envelope (mirrors cmd/portal/main.go).
	strictHandler := openapi.NewStrictHandlerWithOptions(&accountsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	// Build an api wrapper for path-param routes.
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictHandler,
		ErrorHandlerFunc: httperr.WriteBadRequest,
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

		// Get org: auth + org-membership check is performed inside the handler.
		r.Get("/api/orgs/{orgID}", apiWrapper.GetOrg)

		// Patch org: auth + creator-role check is performed inside the handler.
		r.Patch("/api/orgs/{orgID}", apiWrapper.PatchOrg)
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
// GET /api/orgs/{orgID}
// ---------------------------------------------------------------------------

func TestGetOrg_NoBearer_Returns401(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	org := seedOrg(t, env.s, "GetOrg401Org", "getorg-401")
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID, "")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestGetOrg_NotOrgMember_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	outsider := seedAccount(t, env.s, "outsider-get@example.com")
	org := seedOrg(t, env.s, "GetOrg403Org", "getorg-403")

	tok := env.bearerToken(t, outsider.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID, tok)

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestGetOrg_CreatorSuccess(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-get@example.com")
	org := seedOrg(t, env.s, "GetOrgSuccessOrg", "getorg-200")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	tok := env.bearerToken(t, creator.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID, tok)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["id"] != org.ID {
		t.Errorf("id: got %v, want %s", body["id"], org.ID)
	}
	if body["session_invite_policy"] != "members_only" {
		t.Errorf("session_invite_policy: got %v, want members_only", body["session_invite_policy"])
	}
}

func TestGetOrg_MemberSuccess(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-getm@example.com")
	member := seedAccount(t, env.s, "member-getm@example.com")
	org := seedOrg(t, env.s, "GetOrgMemberOrg", "getorg-member")
	seedMember(t, env.s, org.ID, creator.ID, "creator")
	seedMember(t, env.s, org.ID, member.ID, "member")

	tok := env.bearerToken(t, member.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID, tok)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
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

// TestCreateOrgInvite_SenderError_Returns503DepSMTPUnavailable verifies
// the org-invite path wraps Sender failures into the dep envelope
// (HTTP 503, error=dep.smtp_unavailable, Retry-After:5).
func TestCreateOrgInvite_SenderError_Returns503DepSMTPUnavailable(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "boss-fail@example.com")
	org := seedOrg(t, env.s, "FailOrg", "failorg")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	// Inject a transient sender failure. The handler must wrap it with
	// deperr.WrapSMTP so the translator surfaces the typed envelope.
	env.sender.mu.Lock()
	env.sender.err = fmt.Errorf("%w: forced", senders.ErrTransient)
	env.sender.mu.Unlock()

	tok := env.bearerToken(t, creator.ID)
	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/invites", tok,
		map[string]any{"email": "invitee@example.com"})

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: want application/json; charset=utf-8, got %q", ct)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "5" {
		t.Errorf("Retry-After: want 5, got %q", ra)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if code, _ := body["error"].(string); code != "dep.smtp_unavailable" {
		t.Errorf("error code: want dep.smtp_unavailable, got %q", code)
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

// ---------------------------------------------------------------------------
// PATCH /api/orgs/{orgID}
// ---------------------------------------------------------------------------

func TestPatchOrg_NoBearer_Returns401(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	org := seedOrg(t, env.s, "PatchOrg401Org", "patchorg-401")
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, "",
		map[string]any{"session_invite_policy": "open"})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPatchOrg_NotOrgMember_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	outsider := seedAccount(t, env.s, "outsider-patch@example.com")
	org := seedOrg(t, env.s, "PatchOrg403Org", "patchorg-403")

	tok := env.bearerToken(t, outsider.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "open"})

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}

func TestPatchOrg_NonCreatorMember_Returns403(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	member := seedAccount(t, env.s, "regular-patch@example.com")
	org := seedOrg(t, env.s, "PatchOrg403MemberOrg", "patchorg-403m")
	seedMember(t, env.s, org.ID, member.ID, "member")

	tok := env.bearerToken(t, member.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "open"})

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if code, _ := body["error"].(string); code != "auth.insufficient_permission" {
		t.Errorf("error code: want auth.insufficient_permission, got %q", code)
	}
}

func TestPatchOrg_CreatorSuccess_PolicePersists(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-patch@example.com")
	org := seedOrg(t, env.s, "PatchOrg200Org", "patchorg-200")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	// Default policy is members_only; flip to open.
	tok := env.bearerToken(t, creator.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "open"})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if body["id"] != org.ID {
		t.Errorf("id: got %v, want %s", body["id"], org.ID)
	}
	if body["session_invite_policy"] != "open" {
		t.Errorf("session_invite_policy: got %v, want open", body["session_invite_policy"])
	}

	// Verify persistence via the store.
	updated, err := env.s.GetOrgByID(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if updated.SessionInvitePolicy != "open" {
		t.Errorf("store: session_invite_policy: got %q, want open", updated.SessionInvitePolicy)
	}
}

func TestPatchOrg_InvalidPolicyValue_Returns400(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-bad@example.com")
	org := seedOrg(t, env.s, "PatchOrg400Org", "patchorg-400")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	tok := env.bearerToken(t, creator.ID)
	// Use a raw JSON body with an invalid enum value; oapi-codegen rejects it
	// at the request-parsing layer via RequestErrorHandlerFunc → 400.
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "garbage"})

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestPatchOrg_Grandfather verifies the grandfather invariant: flipping
// session_invite_policy from "open" to "members_only" does NOT remove
// session_members rows for accounts that already joined as guests. Their
// membership is preserved by design.
func TestPatchOrg_Grandfather(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-gf@example.com")
	guest := seedAccount(t, env.s, "guest-gf@example.com")
	org := seedOrg(t, env.s, "GrandfatherOrg", "grandfather-org")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	// Set policy to open.
	if err := env.s.UpdateOrgSessionInvitePolicy(context.Background(), store.UpdateOrgSessionInvitePolicyParams{
		ID:                  org.ID,
		SessionInvitePolicy: "open",
	}); err != nil {
		t.Fatalf("set policy open: %v", err)
	}

	// Seed a session and add the guest as a session member (simulating an
	// open-policy join that already happened).
	sess, err := env.s.CreateSession(context.Background(), store.CreateSessionParams{
		ID:            uuid.New().String(),
		OrgID:         org.ID,
		Name:          "grandfather-session",
		Goal:          "test",
		WritableScope: "[]",
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := env.s.AddSessionMember(context.Background(), store.AddSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sess.ID,
		AccountID: guest.ID,
		Role:      "member",
		JoinedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}

	// Now creator flips policy back to members_only.
	tok := env.bearerToken(t, creator.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "members_only"})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify policy was updated.
	updated, err := env.s.GetOrgByID(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if updated.SessionInvitePolicy != "members_only" {
		t.Errorf("policy: got %q, want members_only", updated.SessionInvitePolicy)
	}

	// Verify the guest's session_members row was NOT removed (grandfather).
	sm, err := env.s.GetSessionMember(context.Background(), store.GetSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sess.ID,
		AccountID: guest.ID,
	})
	if err != nil {
		t.Fatalf("get session member (grandfather check): %v", err)
	}
	if sm.AccountID != guest.ID {
		t.Errorf("grandfather: expected session member to persist, got account %s", sm.AccountID)
	}
}

// TestPatchOrg_ProtectedOrg_Returns409 verifies the defense-in-depth guard:
// a creator calling PATCH on a protected org is rejected with 409 org.protected,
// regardless of which fields are being mutated. This mirrors the playground org
// case where session_invite_policy="open" is load-bearing for anonymous joins.
func TestPatchOrg_ProtectedOrg_Returns409(t *testing.T) {
	env := newOrgsMembersTestEnv(t)

	creator := seedAccount(t, env.s, "creator-protected@example.com")

	// Seed a protected org (org_protected=true, session_invite_policy=open),
	// matching how the playground org is provisioned at startup.
	protectedOrg, err := env.s.CreateProtectedOrg(context.Background(), store.CreateProtectedOrgParams{
		ID:        "protected-org-id-test",
		Name:      "PlaygroundOrg",
		Slug:      "playground-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed protected org: %v", err)
	}
	seedMember(t, env.s, protectedOrg.ID, creator.ID, "creator")

	tok := env.bearerToken(t, creator.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+protectedOrg.ID, tok,
		map[string]any{"session_invite_policy": "members_only"})

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if code, _ := body["error"].(string); code != "org.protected" {
		t.Errorf("error code: want org.protected, got %q", code)
	}

	// Verify the policy was NOT mutated — the org remains open.
	reloaded, err := env.s.GetOrgByID(context.Background(), protectedOrg.ID)
	if err != nil {
		t.Fatalf("reload org: %v", err)
	}
	if reloaded.SessionInvitePolicy != "open" {
		t.Errorf("session_invite_policy mutated on protected org: got %q, want open", reloaded.SessionInvitePolicy)
	}
}

// ---------------------------------------------------------------------------
// Dep-failure tests: authfail transient DB path
// ---------------------------------------------------------------------------

// failingGetOrgMemberStore wraps a real store and returns a transient error
// from GetOrgMember, simulating a DB connection failure during org-membership
// auth. This exercises the fail.Err branch in GetOrg and PatchOrg, which must
// wrap the error with deperr.WrapDBIfTransient so httperr.WriteFromError emits
// the canonical dep.db_unavailable envelope.
type failingGetOrgMemberStore struct {
	store.Store
}

func (f *failingGetOrgMemberStore) GetOrgMember(_ context.Context, _ store.GetOrgMemberParams) (store.OrgMember, error) {
	return store.OrgMember{}, fmt.Errorf("conn refused")
}

// newOrgsDepFailTestEnv returns a test env wired with the failing store.
// The real store is still used for seeding and token validation; only
// GetOrgMember is overridden to simulate a transient DB error.
func newOrgsDepFailTestEnv(t *testing.T) (*orgsMembersTestEnv, store.Store) {
	t.Helper()
	realStore := openStore(t)
	failing := &failingGetOrgMemberStore{Store: realStore}

	svc := tokens.New(realStore) // tokens validated against real store
	sender := &captureSenderOrgs{}
	h := accounts.New(failing, sender, "https://portal.example.com")
	strictHandler := openapi.NewStrictHandlerWithOptions(&accountsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictHandler,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		// BearerMiddleware must use the real store so token validation passes.
		r.Use(tokens.BearerMiddleware(svc))
		r.Get("/api/orgs/{orgID}", apiWrapper.GetOrg)
		r.Patch("/api/orgs/{orgID}", apiWrapper.PatchOrg)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	env := &orgsMembersTestEnv{srv: srv, svc: svc, s: realStore, sender: sender}
	return env, realStore
}

func assertDepDBUnavailable(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type: want application/json; charset=utf-8, got %q", ct)
	}
	if ra := resp.Header.Get("Retry-After"); ra != "2" {
		t.Errorf("Retry-After: want 2, got %q", ra)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if code, _ := body["error"].(string); code != "dep.db_unavailable" {
		t.Errorf("error code: want dep.db_unavailable, got %q", code)
	}
}

// TestGetOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable verifies that
// when GetOrgMember returns a transient DB error during RequireOrgMember,
// GetOrg wraps it with deperr.WrapDBIfTransient and the response is the
// canonical dep.db_unavailable envelope (503 + Retry-After: 2).
func TestGetOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable(t *testing.T) {
	env, realStore := newOrgsDepFailTestEnv(t)

	// Seed account and org so the request gets past BearerMiddleware and into
	// the handler where RequireOrgMember will hit the failing GetOrgMember.
	acc := seedAccount(t, realStore, "getorg-depfail@example.com")
	org := seedOrg(t, realStore, "GetOrgDepFailOrg", "getorg-depfail")

	tok := env.bearerToken(t, acc.ID)
	resp := getJSON(t, env.srv, "/api/orgs/"+org.ID, tok)

	assertDepDBUnavailable(t, resp)
}

// TestPatchOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable verifies that
// when GetOrgMember returns a transient DB error during RequireOrgMember,
// PatchOrg wraps it with deperr.WrapDBIfTransient and the response is the
// canonical dep.db_unavailable envelope (503 + Retry-After: 2).
func TestPatchOrg_DBTransientOnAuthLookup_Returns503DepDBUnavailable(t *testing.T) {
	env, realStore := newOrgsDepFailTestEnv(t)

	acc := seedAccount(t, realStore, "patchorg-depfail@example.com")
	org := seedOrg(t, realStore, "PatchOrgDepFailOrg", "patchorg-depfail")

	tok := env.bearerToken(t, acc.ID)
	resp := patchJSON(t, env.srv, "/api/orgs/"+org.ID, tok,
		map[string]any{"session_invite_policy": "open"})

	assertDepDBUnavailable(t, resp)
}

// Ensure the import is used.
var _ = openapi_types.Email("")
