package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// captureSender captures the last email Send call for assertion.
type captureSender struct {
	mu        sync.Mutex
	recipient string
	subject   string
	body      string
	calls     int
	err       error // inject to simulate failure
}

func (c *captureSender) Send(_ context.Context, recipient, subject, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recipient = recipient
	c.subject = subject
	c.body = body
	c.calls++
	return c.err
}

func (c *captureSender) lastRecipient() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recipient
}

func (c *captureSender) lastBody() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

// ---------------------------------------------------------------------------
// Test setup
// ---------------------------------------------------------------------------

// magicLinkTestEnv holds a running httptest.Server wired with the magic-link
// endpoints plus a pointer to the captureSender.
type magicLinkTestEnv struct {
	srv    *httptest.Server
	sender *captureSender
}

func newMagicLinkTestEnv(t *testing.T) *magicLinkTestEnv {
	t.Helper()
	s := openStore(t)
	sender := &captureSender{}
	tokenSvc := tokens.New(s)
	handler := auth.NewMagicLinkHandler(s, tokenSvc, sender, "https://portal.example.com")

	// Build a full strict server that satisfies the openapi interface.
	fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: handler}
	strictAPI := openapi.NewStrictHandler(fullHandler, nil)

	r := chi.NewRouter()
	r.Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
	r.Post("/api/auth/magic-link/exchange", strictAPI.ExchangeMagicLink)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &magicLinkTestEnv{srv: srv, sender: sender}
}

// magicLinkOnlyStrict wraps MagicLinkHandler and panics on unrelated methods.
type magicLinkOnlyStrict struct {
	*auth.MagicLinkHandler
}

func (m *magicLinkOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("RefreshToken: not wired in this test")
}

func (m *magicLinkOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("RevokeToken: not wired in this test")
}

func (m *magicLinkOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("StartOAuth: not wired in this test")
}

func (m *magicLinkOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("OauthCallback: not wired in this test")
}

func (m *magicLinkOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("GetMe: not wired in this test")
}

func (m *magicLinkOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("CreateOrg: not wired in this test")
}

func (m *magicLinkOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("ListOrgMembers: not wired in this test")
}

func (m *magicLinkOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("CreateOrgInvite: not wired in this test")
}

func (m *magicLinkOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("AcceptOrgInvite: not wired in this test")
}
func (m *magicLinkOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("CreateSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("PatchSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("FinalizeSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("AbandonSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("ListSessions: not wired in this test")
}
func (m *magicLinkOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("GetSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("ListSessionRefs: not wired in this test")
}
func (m *magicLinkOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("GetSessionDigest: not wired in this test")
}
func (m *magicLinkOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("InviteToSession: not wired in this test")
}
func (m *magicLinkOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("AcceptSessionInvite: not wired in this test")
}
func (m *magicLinkOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("RemoveSessionMember: not wired in this test")
}
func (m *magicLinkOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("ListComments: not wired in this test")
}
func (m *magicLinkOnlyStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("CreateComment: not wired in this test")
}
func (m *magicLinkOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("ResolveComment: not wired in this test")
}
func (m *magicLinkOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("GetSessionFile: not wired in this test")
}
func (m *magicLinkOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("UpsertRefMode: not wired in this test")
}
func (m *magicLinkOnlyStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("AcquireFinalizeLock: not wired in this test")
}
func (m *magicLinkOnlyStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("PatchFinalizeLock: not wired in this test")
}
func (m *magicLinkOnlyStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("ReleaseFinalizeLock: not wired in this test")
}

var _ openapi.StrictServerInterface = (*magicLinkOnlyStrict)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func postJSONBody(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeJSONResponse(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

// requestAndExtractToken issues a magic-link request and returns the raw token
// extracted from the captured email body.
func requestAndExtractToken(t *testing.T, env *magicLinkTestEnv, email string) string {
	t.Helper()
	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/request",
		map[string]string{"email": email})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("request: want 204, got %d", resp.StatusCode)
	}
	body := env.sender.lastBody()
	// Extract token from "?token=<hex>"
	const prefix = "?token="
	idx := strings.Index(body, prefix)
	if idx == -1 {
		t.Fatalf("body missing token URL: %q", body)
	}
	rest := body[idx+len(prefix):]
	// Token ends at first whitespace or newline.
	end := strings.IndexAny(rest, " \t\n\r")
	if end != -1 {
		rest = rest[:end]
	}
	return rest
}

// ---------------------------------------------------------------------------
// POST /api/auth/magic-link/request tests
// ---------------------------------------------------------------------------

func TestRequestMagicLink_ValidEmail_Returns204(t *testing.T) {
	env := newMagicLinkTestEnv(t)

	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/request",
		map[string]string{"email": "user@example.com"})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
	if env.sender.calls != 1 {
		t.Errorf("want 1 email send, got %d", env.sender.calls)
	}
}

func TestRequestMagicLink_SenderCalledWithCorrectRecipient(t *testing.T) {
	env := newMagicLinkTestEnv(t)

	email := "specific@example.com"
	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/request",
		map[string]string{"email": email})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
	if got := env.sender.lastRecipient(); got != email {
		t.Errorf("sender recipient: want %q, got %q", email, got)
	}
}

func TestRequestMagicLink_SentBodyContainsURL(t *testing.T) {
	env := newMagicLinkTestEnv(t)

	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/request",
		map[string]string{"email": "link@example.com"})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}

	body := env.sender.lastBody()
	if !strings.Contains(body, "https://portal.example.com/auth/magic-link?token=") {
		t.Errorf("email body missing magic-link URL; got: %q", body)
	}
}

func TestRequestMagicLink_SenderError_Returns500(t *testing.T) {
	env := newMagicLinkTestEnv(t)
	env.sender.err = fmt.Errorf("%w: test transient error", senders.ErrTransient)

	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/request",
		map[string]string{"email": "fail@example.com"})

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500 on sender error, got %d", resp.StatusCode)
	}
}

func TestRequestMagicLink_SubjectIsCorrect(t *testing.T) {
	env := newMagicLinkTestEnv(t)
	_ = requestAndExtractToken(t, env, "subj@example.com")

	if env.sender.subject != "Sign in to jamsesh" {
		t.Errorf("subject: want %q, got %q", "Sign in to jamsesh", env.sender.subject)
	}
}

// ---------------------------------------------------------------------------
// POST /api/auth/magic-link/exchange tests
// ---------------------------------------------------------------------------

func TestExchangeMagicLink_ValidToken_Returns200WithTokenPair(t *testing.T) {
	env := newMagicLinkTestEnv(t)
	token := requestAndExtractToken(t, env, "exchange@example.com")

	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	body := decodeJSONResponse(t, resp)
	for _, field := range []string{"access_token", "refresh_token", "access_expires_at", "refresh_expires_at"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing field %q", field)
		}
	}
}

func TestExchangeMagicLink_SecondUse_Returns401(t *testing.T) {
	env := newMagicLinkTestEnv(t)
	token := requestAndExtractToken(t, env, "reuse@example.com")

	// First use: success.
	resp1 := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token})
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first exchange: want 200, got %d", resp1.StatusCode)
	}

	// Second use of same token: must fail with 401.
	resp2 := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token})
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second exchange: want 401, got %d", resp2.StatusCode)
	}
}

func TestExchangeMagicLink_SecondUse_ErrorCodeIsAlreadyUsed(t *testing.T) {
	env := newMagicLinkTestEnv(t)
	token := requestAndExtractToken(t, env, "used-check@example.com")

	// First use.
	postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token})

	// Second use: check error code.
	resp2 := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token})
	body := decodeJSONResponse(t, resp2)
	if code, _ := body["error"].(string); code != "auth.invalid_token" {
		t.Errorf("error code: want %q, got %q", "auth.invalid_token", code)
	}
}

func TestExchangeMagicLink_InvalidToken_Returns401(t *testing.T) {
	env := newMagicLinkTestEnv(t)

	resp := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa1111bbbb2222"})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid token, got %d", resp.StatusCode)
	}
}

func TestExchangeMagicLink_ProvisionedAccount_IsIdempotent(t *testing.T) {
	env := newMagicLinkTestEnv(t)

	// Two magic-link exchanges from the same email should both succeed and
	// both issue tokens (the account is found, not re-created).

	// First token.
	token1 := requestAndExtractToken(t, env, "idempotent@example.com")
	resp1 := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token1})
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first exchange: want 200, got %d", resp1.StatusCode)
	}

	// Reset sender state.
	env.sender.mu.Lock()
	env.sender.calls = 0
	env.sender.mu.Unlock()

	// Second token (different, but same email).
	token2 := requestAndExtractToken(t, env, "idempotent@example.com")
	resp2 := postJSONBody(t, env.srv, "/api/auth/magic-link/exchange",
		map[string]string{"token": token2})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second exchange: want 200, got %d", resp2.StatusCode)
	}

	// Both responses should contain valid token pairs (different access tokens).
	body1 := decodeJSONResponse(t, resp1)
	body2 := decodeJSONResponse(t, resp2)
	if body1["access_token"] == body2["access_token"] {
		t.Error("expected different access tokens for two separate logins")
	}
}
