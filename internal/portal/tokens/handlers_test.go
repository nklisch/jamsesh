package tokens_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// testEnv holds the shared objects for a single handler test case.
type testEnv struct {
	store store.Store
	svc   tokens.Service
	srv   *httptest.Server
}

// tokensOnlyHandler wraps tokens.Handler and satisfies the full
// openapi.StrictServerInterface by panicking on methods not owned by this
// package. This lets the handler tests remain independent of the auth package.
type tokensOnlyHandler struct {
	*tokens.Handler
}

func (t *tokensOnlyHandler) ExchangeMagicLink(ctx context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("ExchangeMagicLink: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) RequestMagicLink(ctx context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("RequestMagicLink: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("StartOAuth: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("OauthCallback: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("GetMe: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("CreateOrg: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("ListOrgMembers: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("CreateOrgInvite: not implemented in token handler tests")
}

func (t *tokensOnlyHandler) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("AcceptOrgInvite: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("CreateSession: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("PatchSession: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("FinalizeSession: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("AbandonSession: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("ListSessions: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("GetSession: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("ListSessionRefs: not implemented in token handler tests")
}
func (t *tokensOnlyHandler) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("GetSessionDigest: not implemented in token handler tests")
}

var _ openapi.StrictServerInterface = (*tokensOnlyHandler)(nil)

// newTestEnv creates a fresh in-memory SQLite store, builds the tokens.Service
// and handler stack, and starts an httptest.Server with the same public/
// authenticated route split used in cmd/portal/main.go.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	s := openStore(t)
	svc := tokens.New(s)
	h := tokens.NewHandler(svc)
	strictAPI := openapi.NewStrictHandler(&tokensOnlyHandler{h}, nil)

	r := chi.NewRouter()

	// Public: POST /api/auth/refresh (no Bearer middleware)
	r.Group(func(r chi.Router) {
		r.Post("/api/auth/refresh", strictAPI.RefreshToken)
	})

	// Authenticated: POST /api/auth/revoke (Bearer required)
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Post("/api/auth/revoke", strictAPI.RevokeToken)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testEnv{store: s, svc: svc, srv: srv}
}

// mustIssue creates an account and issues a token pair for it.
func (e *testEnv) mustIssue(t *testing.T, email string) (store.Account, tokens.Pair) {
	t.Helper()
	ctx := context.Background()
	acc := mustCreateAccount(t, ctx, e.store, email)
	pair, err := e.svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue for %q: %v", email, err)
	}
	return acc, pair
}

// postJSON posts a JSON body to path and returns the response.
func postJSON(t *testing.T, srv *httptest.Server, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// decodeJSON decodes the response body as JSON into a map.
func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

// ---------------------------------------------------------------------------
// POST /api/auth/refresh tests
// ---------------------------------------------------------------------------

func TestHandler_RefreshToken_ValidRefreshToken_Returns200WithTokenPair(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "refresh-valid@example.com")

	resp := postJSON(t, env.srv, "/api/auth/refresh",
		map[string]string{"refresh_token": pair.RefreshToken}, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	body := decodeJSON(t, resp)
	for _, field := range []string{"access_token", "refresh_token", "access_expires_at", "refresh_expires_at"} {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing field %q", field)
		}
	}
	if body["access_token"] == "" {
		t.Error("access_token is empty")
	}
}

func TestHandler_RefreshToken_InvalidToken_Returns401(t *testing.T) {
	env := newTestEnv(t)

	resp := postJSON(t, env.srv, "/api/auth/refresh",
		map[string]string{"refresh_token": "bogus"}, nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}

	body := decodeJSON(t, resp)
	if code, ok := body["error"].(string); !ok || code == "" {
		t.Errorf("response missing 'error' field: %v", body)
	}
}

func TestHandler_RefreshToken_AccessTokenRejected_Returns401(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "refresh-access@example.com")

	// Supplying an access token where a refresh token is required.
	resp := postJSON(t, env.srv, "/api/auth/refresh",
		map[string]string{"refresh_token": pair.AccessToken}, nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestHandler_RefreshToken_UsedTokenRejected_Returns401(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "refresh-reuse@example.com")

	// First call succeeds.
	resp := postJSON(t, env.srv, "/api/auth/refresh",
		map[string]string{"refresh_token": pair.RefreshToken}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first refresh: want 200, got %d", resp.StatusCode)
	}

	// Second call with the same (now revoked) refresh token must fail.
	resp2 := postJSON(t, env.srv, "/api/auth/refresh",
		map[string]string{"refresh_token": pair.RefreshToken}, nil)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused refresh token: want 401, got %d", resp2.StatusCode)
	}
}

func TestHandler_RefreshToken_ExpiredToken_Returns401(t *testing.T) {
	// Build a test env with an injectable clock.
	s := openStore(t)
	clk := &fakeClock{t: time.Now().UTC()}
	svc := tokens.NewWithClock(s, clk)
	h := tokens.NewHandler(svc)
	strictAPI := openapi.NewStrictHandler(&tokensOnlyHandler{h}, nil)

	r := chi.NewRouter()
	r.Post("/api/auth/refresh", strictAPI.RefreshToken)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	acc := mustCreateAccount(t, ctx, s, "refresh-expired@example.com")
	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Advance past RefreshTokenTTL.
	clk.advance(tokens.RefreshTokenTTL + time.Second)

	resp := postJSON(t, srv, "/api/auth/refresh",
		map[string]string{"refresh_token": pair.RefreshToken}, nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired refresh token: want 401, got %d", resp.StatusCode)
	}

	body := decodeJSON(t, resp)
	if code, _ := body["error"].(string); code != "auth.expired_token" {
		t.Errorf("error code: want auth.expired_token, got %q", code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/auth/revoke tests
// ---------------------------------------------------------------------------

func TestHandler_RevokeToken_WithBearer_Returns204(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "revoke-bearer@example.com")

	resp := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]string{"token": pair.RefreshToken},
		map[string]string{"Authorization": "Bearer " + pair.AccessToken})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

func TestHandler_RevokeToken_WithoutBearer_Returns401(t *testing.T) {
	env := newTestEnv(t)

	resp := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]string{"token": "any"}, nil)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestHandler_RevokeToken_InvalidBearer_Returns401(t *testing.T) {
	env := newTestEnv(t)

	resp := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]string{"token": "any"},
		map[string]string{"Authorization": "Bearer totally-invalid"})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestHandler_RevokeToken_RevokeAll_BothTokensInvalid(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "revoke-all@example.com")
	ctx := context.Background()

	resp := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]any{"token": pair.AccessToken, "revoke_all": true},
		map[string]string{"Authorization": "Bearer " + pair.AccessToken})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}

	// Access token invalid.
	if _, err := env.svc.Validate(ctx, pair.AccessToken); err == nil {
		t.Error("access token still valid after revoke_all")
	}
	// Refresh token also invalid.
	if _, err := env.svc.Validate(ctx, pair.RefreshToken); err == nil {
		t.Error("refresh token still valid after revoke_all")
	}
}

func TestHandler_RevokeToken_RevokedBearerRejectsSubsequentRequests(t *testing.T) {
	env := newTestEnv(t)
	_, pair := env.mustIssue(t, "revoke-then-try@example.com")

	// Revoke the access token (the bearer).
	resp := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]string{"token": pair.AccessToken},
		map[string]string{"Authorization": "Bearer " + pair.AccessToken})

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}

	// Subsequent request with the same revoked bearer must fail.
	resp2 := postJSON(t, env.srv, "/api/auth/revoke",
		map[string]string{"token": pair.RefreshToken},
		map[string]string{"Authorization": "Bearer " + pair.AccessToken})

	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked bearer: want 401, got %d", resp2.StatusCode)
	}
}
