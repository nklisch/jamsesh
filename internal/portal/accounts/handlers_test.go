package accounts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/accounts"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func openStore(t *testing.T) store.Store {
	t.Helper()
	s, _, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedAccount(t *testing.T, s store.Store, email string) store.Account {
	t.Helper()
	acc, err := s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          uuid.New().String(),
		Email:       email,
		DisplayName: strings.Split(email, "@")[0],
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed account %s: %v", email, err)
	}
	return acc
}

func seedOrg(t *testing.T, s store.Store, name, slug string) store.Org {
	t.Helper()
	org, err := s.CreateOrg(context.Background(), store.CreateOrgParams{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed org %s: %v", name, err)
	}
	return org
}

func seedMember(t *testing.T, s store.Store, orgID, accountID, role string) {
	t.Helper()
	if err := s.AddOrgMember(context.Background(), store.AddOrgMemberParams{
		OrgID:     orgID,
		AccountID: accountID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

// noopSender is a Sender that does nothing, for tests that don't exercise email.
type noopSender struct{}

func (n *noopSender) Send(_ context.Context, _, _, _ string) error { return nil }

// accountsOnlyStrict wraps accounts.Handler and panics on methods it doesn't own.
type accountsOnlyStrict struct {
	*accounts.Handler
}

func (a *accountsOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("ExchangeMagicLink: not wired in accounts tests")
}
func (a *accountsOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("RequestMagicLink: not wired in accounts tests")
}
func (a *accountsOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("OauthCallback: not wired in accounts tests")
}
func (a *accountsOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("StartOAuth: not wired in accounts tests")
}
func (a *accountsOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("RefreshToken: not wired in accounts tests")
}
func (a *accountsOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("RevokeToken: not wired in accounts tests")
}
func (a *accountsOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("CreateSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("PatchSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("FinalizeSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("AbandonSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("ListSessions: not wired in accounts tests")
}
func (a *accountsOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("GetSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("ListSessionRefs: not wired in accounts tests")
}
func (a *accountsOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("GetSessionDigest: not wired in accounts tests")
}
func (a *accountsOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("InviteToSession: not wired in accounts tests")
}
func (a *accountsOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("AcceptSessionInvite: not wired in accounts tests")
}
func (a *accountsOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("RemoveSessionMember: not wired in accounts tests")
}
func (a *accountsOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("ListComments: not wired in accounts tests")
}
func (a *accountsOnlyStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("CreateComment: not wired in accounts tests")
}
func (a *accountsOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("ResolveComment: not wired in accounts tests")
}
func (a *accountsOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("GetSessionFile: not wired in accounts tests")
}
func (a *accountsOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("UpsertRefMode: not wired in accounts tests")
}
func (a *accountsOnlyStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("AcquireFinalizeLock: not wired in accounts tests")
}
func (a *accountsOnlyStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("PatchFinalizeLock: not wired in accounts tests")
}
func (a *accountsOnlyStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("ReleaseFinalizeLock: not wired in accounts tests")
}
func (a *accountsOnlyStrict) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	panic("GetFinalizePlan: not wired in accounts tests")
}
func (a *accountsOnlyStrict) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	panic("IssueFetchToken: not wired in accounts tests")
}
func (a *accountsOnlyStrict) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	panic("MarkSessionShipped: not wired in accounts tests")
}
func (a *accountsOnlyStrict) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	panic("GetSessionInvite: not wired in accounts tests")
}

// GetOrg is owned by the accounts handler; delegate directly.
func (a *accountsOnlyStrict) GetOrg(ctx context.Context, req openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	return a.Handler.GetOrg(ctx, req)
}

var _ openapi.StrictServerInterface = (*accountsOnlyStrict)(nil)

type accountsTestEnv struct {
	srv *httptest.Server
	svc tokens.Service
	s   store.Store
}

func newAccountsTestEnv(t *testing.T) *accountsTestEnv {
	t.Helper()
	s := openStore(t)
	return newAccountsTestEnvWithStore(t, s)
}

// newAccountsTestEnvWithStore lets dep-failure tests inject a wrapping store
// that returns transient DB errors from selected methods. The strict-handler
// translator is wired explicitly so deperr-wrapped errors surface as typed
// envelopes instead of the default plain-text 500 (mirrors cmd/portal/main.go).
func newAccountsTestEnvWithStore(t *testing.T, s store.Store) *accountsTestEnv {
	t.Helper()
	svc := tokens.New(s)
	h := accounts.New(s, &noopSender{}, "https://portal.example.com")
	strictAPI := openapi.NewStrictHandlerWithOptions(&accountsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Get("/api/me", strictAPI.GetMe)
		r.Post("/api/orgs", strictAPI.CreateOrg)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &accountsTestEnv{srv: srv, svc: svc, s: s}
}

func (e *accountsTestEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

func getJSON(t *testing.T, srv *httptest.Server, path, bearer string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func postJSON(t *testing.T, srv *httptest.Server, path, bearer string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func patchJSON(t *testing.T, srv *httptest.Server, path, bearer string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// ---------------------------------------------------------------------------
// GET /api/me tests
// ---------------------------------------------------------------------------

func TestGetMe_HappyPath(t *testing.T) {
	env := newAccountsTestEnv(t)

	acc := seedAccount(t, env.s, "alice@example.com")
	org := seedOrg(t, env.s, "alice-org", "alice-org")
	seedMember(t, env.s, org.ID, acc.ID, "creator")

	tok := env.bearerToken(t, acc.ID)
	resp := getJSON(t, env.srv, "/api/me", tok)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["id"] != acc.ID {
		t.Errorf("id: got %v, want %s", body["id"], acc.ID)
	}
	if body["email"] != acc.Email {
		t.Errorf("email: got %v, want %s", body["email"], acc.Email)
	}
	if body["display_name"] != acc.DisplayName {
		t.Errorf("display_name: got %v, want %s", body["display_name"], acc.DisplayName)
	}

	orgs, ok := body["orgs"].([]any)
	if !ok || len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %v", body["orgs"])
	}
	m := orgs[0].(map[string]any)
	if m["id"] != org.ID {
		t.Errorf("org id: got %v, want %s", m["id"], org.ID)
	}
	if m["role"] != "creator" {
		t.Errorf("org role: got %v, want creator", m["role"])
	}
}

func TestGetMe_MultipleOrgs(t *testing.T) {
	env := newAccountsTestEnv(t)

	acc := seedAccount(t, env.s, "bob@example.com")
	org1 := seedOrg(t, env.s, "org-one", "org-one")
	org2 := seedOrg(t, env.s, "org-two", "org-two")
	seedMember(t, env.s, org1.ID, acc.ID, "creator")
	seedMember(t, env.s, org2.ID, acc.ID, "member")

	tok := env.bearerToken(t, acc.ID)
	resp := getJSON(t, env.srv, "/api/me", tok)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	orgs := body["orgs"].([]any)
	if len(orgs) != 2 {
		t.Errorf("expected 2 orgs, got %d", len(orgs))
	}
}

func TestGetMe_NoAuth_Returns401(t *testing.T) {
	env := newAccountsTestEnv(t)

	resp := getJSON(t, env.srv, "/api/me", "")

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs tests
// ---------------------------------------------------------------------------

func TestCreateOrg_HappyPath(t *testing.T) {
	env := newAccountsTestEnv(t)

	acc := seedAccount(t, env.s, "carol@example.com")
	tok := env.bearerToken(t, acc.ID)

	resp := postJSON(t, env.srv, "/api/orgs", tok, map[string]any{"name": "New Org"})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	if body["name"] != "New Org" {
		t.Errorf("name: got %v, want 'New Org'", body["name"])
	}
	if body["slug"] != "new-org" {
		t.Errorf("slug: got %v, want 'new-org'", body["slug"])
	}
	if body["id"] == nil || body["id"] == "" {
		t.Error("expected non-empty id")
	}

	// Verify the authenticated account is now a creator member.
	orgID := body["id"].(string)
	m, err := env.s.GetOrgMember(context.Background(), store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	})
	if err != nil {
		t.Fatalf("get org member: %v", err)
	}
	if m.Role != "creator" {
		t.Errorf("expected role creator, got %s", m.Role)
	}
}

func TestCreateOrg_SlugCollision_AppendsSuffix(t *testing.T) {
	env := newAccountsTestEnv(t)

	acc := seedAccount(t, env.s, "dave@example.com")
	tok := env.bearerToken(t, acc.ID)

	// Pre-create an org that would collide with the slug of "collision-test".
	_, err := env.s.CreateOrg(context.Background(), store.CreateOrgParams{
		ID:        uuid.New().String(),
		Name:      "collision-test",
		Slug:      "collision-test",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("pre-create org: %v", err)
	}

	resp := postJSON(t, env.srv, "/api/orgs", tok, map[string]any{"name": "Collision Test"})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	slug, _ := body["slug"].(string)
	if !strings.HasPrefix(slug, "collision-test-") {
		t.Errorf("expected slug with suffix, got %q", slug)
	}
}

func TestCreateOrg_NoAuth_Returns401(t *testing.T) {
	env := newAccountsTestEnv(t)

	resp := postJSON(t, env.srv, "/api/orgs", "", map[string]any{"name": "No Auth Org"})

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Dep-failure tests
// ---------------------------------------------------------------------------

// failingListOrgsStore wraps a real store and returns a transient error from
// ListOrgsForAccount, simulating a DB connection failure.
type failingListOrgsStore struct {
	store.Store
}

func (f *failingListOrgsStore) ListOrgsForAccount(_ context.Context, _ string) ([]store.Org, error) {
	return nil, errors.New("conn refused")
}

func TestGetMe_DBUnavailable_Returns503DepDBUnavailable(t *testing.T) {
	realStore := openStore(t)
	// Seed an account so BearerMiddleware can validate the token before the
	// handler reaches the failing call site.
	acc := seedAccount(t, realStore, "depfail@example.com")

	failing := &failingListOrgsStore{Store: realStore}
	env := newAccountsTestEnvWithStore(t, failing)

	tok := env.bearerToken(t, acc.ID)
	resp := getJSON(t, env.srv, "/api/me", tok)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got != "2" {
		t.Errorf("expected Retry-After: 2, got %q", got)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "dep.db_unavailable" {
		t.Errorf("expected error=dep.db_unavailable, got %v", body["error"])
	}
}
