package sessionresume_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/sessionresume"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// testEnv harness — mirrors the finalize fetchTokenEnv shape
// ---------------------------------------------------------------------------

type testEnv struct {
	s         store.Store
	clock     *fakeClock
	handler   *sessionresume.Handler
	portalURL string

	orgID  string
	sessID string
	caller store.Account
	// other is an org member but NOT a session member.
	other store.Account
	// nonMember has no org membership at all.
	nonMember store.Account

	callerCtx    context.Context
	otherCtx     context.Context
	nonMemberCtx context.Context
}

// fakeClock is a controllable Now() source.
type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time { return f.t }

// newTestEnv builds a fresh in-memory SQLite store and wires the handler.
func newTestEnv(t *testing.T, portalURL string) *testEnv {
	t.Helper()

	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	clk := &fakeClock{t: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	tokSvc := tokens.New(s)
	handler := sessionresume.NewWithClock(s, tokSvc, portalURL, clk)

	now := clk.Now()
	orgID := ulid.Make().String()
	callerID := ulid.Make().String()
	otherID := ulid.Make().String()
	nonMemberID := ulid.Make().String()
	sessID := ulid.Make().String()

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "srorg", Slug: fmt.Sprintf("srorg-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	caller, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: callerID, Email: fmt.Sprintf("caller-%s@ex.com", callerID[:8]),
		DisplayName: "Caller", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	other, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: otherID, Email: fmt.Sprintf("other-%s@ex.com", otherID[:8]),
		DisplayName: "Other", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	nonMember, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: nonMemberID, Email: fmt.Sprintf("nonmember-%s@ex.com", nonMemberID[:8]),
		DisplayName: "NonMember", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create nonMember: %v", err)
	}

	// caller → org member + session member (creator)
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: callerID, Role: "creator", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add caller org member: %v", err)
	}
	// other → org member only (no session member)
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: otherID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add other org member: %v", err)
	}
	// nonMember → no org membership at all

	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "sr-test", Goal: "resume test",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessID, AccountID: callerID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add caller session member: %v", err)
	}

	return &testEnv{
		s:            s,
		clock:        clk,
		handler:      handler,
		portalURL:    portalURL,
		orgID:        orgID,
		sessID:       sessID,
		caller:       caller,
		other:        other,
		nonMember:    nonMember,
		callerCtx:    tokens.ContextWithAccount(ctx, &caller),
		otherCtx:     tokens.ContextWithAccount(ctx, &other),
		nonMemberCtx: tokens.ContextWithAccount(ctx, &nonMember),
	}
}

// ---------------------------------------------------------------------------
// Shim: sessionresumeOnlyStrict satisfies openapi.StrictServerInterface by
// delegating CreateSessionResume to the real handler and panicking everywhere
// else. This follows the strict-server-partial-handler-shim pattern.
// ---------------------------------------------------------------------------

type sessionresumeOnlyStrict struct {
	*sessionresume.Handler
}

func (h *sessionresumeOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) Logout(_ context.Context, _ openapi.LogoutRequestObject) (openapi.LogoutResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetOrg(_ context.Context, _ openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) PatchOrg(_ context.Context, _ openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) IssueWsTicket(_ context.Context, _ openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) CreatePlaygroundSession(_ context.Context, _ openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) JoinPlaygroundSession(_ context.Context, _ openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetPlaygroundSession(_ context.Context, _ openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetPlaygroundTombstone(_ context.Context, _ openapi.GetPlaygroundTombstoneRequestObject) (openapi.GetPlaygroundTombstoneResponseObject, error) {
	panic("not wired")
}
func (h *sessionresumeOnlyStrict) GetPortalInfo(_ context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
	panic("not wired")
}

var _ openapi.StrictServerInterface = (*sessionresumeOnlyStrict)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCreateSessionResume_NoBearerToken_401 verifies that a request without
// any bearer token (no account in context) returns 401.
func TestCreateSessionResume_NoBearerToken_401(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(context.Background(), openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: env.sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(openapi.CreateSessionResume401JSONResponse); !ok {
		t.Fatalf("expected 401, got %T", resp)
	}
}

// TestCreateSessionResume_NonOrgMember_403 verifies that an account with no
// org membership gets 403.
func TestCreateSessionResume_NonOrgMember_403(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(env.nonMemberCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: env.sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume403JSONResponse)
	if !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
	if !strings.Contains(r.Error, "auth.insufficient_permission") {
		t.Errorf("error code = %q, want auth.insufficient_permission", r.Error)
	}
}

// TestCreateSessionResume_OrgMemberButNotSessionMember_403 verifies that an
// account who is an org member but not a session member gets 403.
func TestCreateSessionResume_OrgMemberButNotSessionMember_403(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(env.otherCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: env.sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume403JSONResponse)
	if !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
	if !strings.Contains(r.Error, "auth.insufficient_permission") {
		t.Errorf("error code = %q, want auth.insufficient_permission", r.Error)
	}
}

// TestCreateSessionResume_UnknownSession_404 verifies that requesting a
// session that doesn't exist returns 404.
func TestCreateSessionResume_UnknownSession_404(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(env.callerCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: ulid.Make().String(), // unknown session ID
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := resp.(openapi.CreateSessionResume404JSONResponse); !ok {
		t.Fatalf("expected 404, got %T", resp)
	}
}

// TestCreateSessionResume_Success_DurableOrg verifies the happy path for a
// durable org session. The response must:
//   - be a 200 JSON response
//   - have expires_in == 60
//   - have session_id matching the requested session
//   - have resume_url containing "#rt=" (raw token in fragment)
//   - have the canonical durable path in resume_url
//   - NOT have a standalone token field
//   - store a HASHED token (sha256 of raw) — not the raw token itself
func TestCreateSessionResume_Success_DurableOrg(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(env.callerCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: env.sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	// expires_in must always be 60.
	if r.ExpiresIn != 60 {
		t.Errorf("expires_in = %d, want 60", r.ExpiresIn)
	}
	// session_id must match.
	if r.SessionId != env.sessID {
		t.Errorf("session_id = %q, want %q", r.SessionId, env.sessID)
	}
	// resume_url must be non-empty and contain "#rt=".
	if r.ResumeUrl == "" {
		t.Fatal("resume_url is empty")
	}
	fragIdx := strings.Index(r.ResumeUrl, "#rt=")
	if fragIdx < 0 {
		t.Fatalf("resume_url %q does not contain '#rt='", r.ResumeUrl)
	}

	// Extract the raw token from the fragment.
	rawToken := r.ResumeUrl[fragIdx+4:]
	if rawToken == "" {
		t.Fatal("raw token in resume_url fragment is empty")
	}

	// Canonical durable path: /orgs/{orgID}/sessions/{sessID}/resume
	wantPathPrefix := fmt.Sprintf("https://portal.example.com/orgs/%s/sessions/%s/resume#rt=", env.orgID, env.sessID)
	if !strings.HasPrefix(r.ResumeUrl, wantPathPrefix) {
		t.Errorf("resume_url = %q, want prefix %q", r.ResumeUrl, wantPathPrefix)
	}

	// SECURITY: stored token_hash must be the sha256 of raw token, NOT the raw token.
	expectedHash := sha256HexOf(rawToken)
	storedToken, err := env.s.GetResumeTokenByHash(context.Background(), expectedHash)
	if err != nil {
		t.Fatalf("GetResumeTokenByHash: %v (raw_token in fragment must hash to the stored row)", err)
	}
	if storedToken.TokenHash == rawToken {
		t.Error("SECURITY: stored token_hash equals raw token — raw token must never be persisted")
	}
	if storedToken.TokenHash != expectedHash {
		t.Errorf("stored token_hash = %q, want sha256(%q) = %q", storedToken.TokenHash, rawToken, expectedHash)
	}

	// The token must be bound to the correct account and session.
	if storedToken.AccountID != env.caller.ID {
		t.Errorf("token account_id = %q, want %q", storedToken.AccountID, env.caller.ID)
	}
	if storedToken.SessionID != env.sessID {
		t.Errorf("token session_id = %q, want %q", storedToken.SessionID, env.sessID)
	}
	if storedToken.OrgID != env.orgID {
		t.Errorf("token org_id = %q, want %q", storedToken.OrgID, env.orgID)
	}

	// expires_at must be now+60s.
	wantExpiry := env.clock.Now().Add(60 * time.Second)
	if !storedToken.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("token expires_at = %s, want %s", storedToken.ExpiresAt, wantExpiry)
	}

	// used_at must be nil (not yet consumed).
	if storedToken.UsedAt != nil {
		t.Errorf("token used_at = %v, want nil (token was minted not consumed)", storedToken.UsedAt)
	}
}

// TestCreateSessionResume_Success_PlaygroundOrg verifies that the playground
// canonical path (/playground/s/{sessionID}/resume) is used when orgID is
// "org_playground".
func TestCreateSessionResume_Success_PlaygroundOrg(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	clk := &fakeClock{t: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	tokSvc := tokens.New(s)
	handler := sessionresume.NewWithClock(s, tokSvc, "https://portal.example.com", clk)

	const playgroundOrgID = "org_playground"
	now := clk.Now()

	if _, err := s.CreateProtectedOrg(ctx, store.CreateProtectedOrgParams{
		ID: playgroundOrgID, Name: "playground", Slug: "playground", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create playground org: %v", err)
	}
	callerID := ulid.Make().String()
	sessID := ulid.Make().String()
	caller, err := s.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
		ID:          callerID,
		Email:       callerID + "@playground.local",
		DisplayName: "anon-1",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create anon account: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: playgroundOrgID, AccountID: callerID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: playgroundOrgID, Name: "pg-test", Goal: "playground",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: playgroundOrgID, SessionID: sessID, AccountID: callerID, Role: "member", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}

	callerCtx := tokens.ContextWithAccount(ctx, &caller)

	resp, err := handler.CreateSessionResume(callerCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     playgroundOrgID,
			SessionId: sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	// Playground canonical path: /playground/s/{sessionID}/resume
	wantPathPrefix := fmt.Sprintf("https://portal.example.com/playground/s/%s/resume#rt=", sessID)
	if !strings.HasPrefix(r.ResumeUrl, wantPathPrefix) {
		t.Errorf("resume_url = %q, want playground prefix %q", r.ResumeUrl, wantPathPrefix)
	}
}

// TestCreateSessionResume_ResponseHasNoStandaloneTokenField verifies that the
// response struct carries no raw token — only resume_url, expires_in,
// session_id.
func TestCreateSessionResume_ResponseHasNoStandaloneTokenField(t *testing.T) {
	env := newTestEnv(t, "https://portal.example.com")

	resp, err := env.handler.CreateSessionResume(env.callerCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     env.orgID,
			SessionId: env.sessID,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T", resp)
	}

	// openapi.SessionResumeResponse only has ResumeUrl, ExpiresIn, SessionId.
	// Confirm the raw token is NOT present as a separate field by ensuring
	// ResumeUrl is the only string field that contains the token.
	fragIdx := strings.Index(r.ResumeUrl, "#rt=")
	if fragIdx < 0 {
		t.Fatal("resume_url has no '#rt=' fragment")
	}
	rawToken := r.ResumeUrl[fragIdx+4:]

	// The raw token must not appear in session_id.
	if r.SessionId == rawToken {
		t.Error("session_id equals raw token — raw token must not leak into session_id")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sha256HexOf returns the SHA-256 hex digest of s.
// Used to verify that the store contains a hash, not the raw token.
func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
