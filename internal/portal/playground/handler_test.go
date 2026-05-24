package playground_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Clock stubs
// ---------------------------------------------------------------------------

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// ---------------------------------------------------------------------------
// Storage stub
// ---------------------------------------------------------------------------

type stubStorage struct {
	repos       map[string]bool
	createError error
}

func newStubStorage() *stubStorage { return &stubStorage{repos: make(map[string]bool)} }

func (s *stubStorage) RepoPath(orgID, sessionID string) string {
	return "/tmp/" + orgID + "/" + sessionID
}
func (s *stubStorage) CreateRepo(_ context.Context, orgID, sessionID string) error {
	if s.createError != nil {
		return s.createError
	}
	s.repos[orgID+"/"+sessionID] = true
	return nil
}
func (s *stubStorage) RemoveRepo(_ context.Context, orgID, sessionID string) error {
	delete(s.repos, orgID+"/"+sessionID)
	return nil
}
func (s *stubStorage) RepoExists(orgID, sessionID string) (bool, error) {
	return s.repos[orgID+"/"+sessionID], nil
}
func (s *stubStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (s *stubStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, store.ErrNotFound
}
func (s *stubStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

// ---------------------------------------------------------------------------
// StrictServerInterface shim — stubs out all methods not under test
// ---------------------------------------------------------------------------

type playgroundOnlyStrict struct {
	*playground.Handler
}

func (h *playgroundOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) IssueWsTicket(_ context.Context, _ openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetOrg(_ context.Context, _ openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) PatchOrg(_ context.Context, _ openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("not wired")
}
func (h *playgroundOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("not wired")
}

// Compile-time check that playgroundOnlyStrict satisfies the full interface.
var _ openapi.StrictServerInterface = (*playgroundOnlyStrict)(nil)

// ---------------------------------------------------------------------------
// Tokens stubs
// ---------------------------------------------------------------------------

// failingTokensService wraps a real tokens.Service and overrides
// IssueAnonymousSessionBearer to return an injected error. All other methods
// delegate to the underlying service unchanged.
type failingTokensService struct {
	real          tokens.Service
	issueErr      error
	lastSessionID string // captures the sessionID arg on every call
}

func (s *failingTokensService) Issue(ctx context.Context, accountID string) (tokens.Pair, error) {
	return s.real.Issue(ctx, accountID)
}
func (s *failingTokensService) IssueShortLived(ctx context.Context, accountID string, ttl time.Duration) (string, time.Time, error) {
	return s.real.IssueShortLived(ctx, accountID, ttl)
}
func (s *failingTokensService) IssueAnonymousSessionBearer(ctx context.Context, sessionID, nickname string, ttl time.Duration) (string, string, time.Time, error) {
	s.lastSessionID = sessionID
	if s.issueErr != nil {
		return "", "", time.Time{}, s.issueErr
	}
	return s.real.IssueAnonymousSessionBearer(ctx, sessionID, nickname, ttl)
}
func (s *failingTokensService) Validate(ctx context.Context, rawToken string) (*store.Account, error) {
	return s.real.Validate(ctx, rawToken)
}
func (s *failingTokensService) Refresh(ctx context.Context, refreshToken string) (tokens.Pair, error) {
	return s.real.Refresh(ctx, refreshToken)
}
func (s *failingTokensService) Revoke(ctx context.Context, callerAccountID string, rawToken string, revokeAll bool) error {
	return s.real.Revoke(ctx, callerAccountID, rawToken, revokeAll)
}

// ---------------------------------------------------------------------------
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	srv   *httptest.Server
	s     store.Store
	svc   tokens.Service
	stor  *stubStorage
	clock playground.Clock
}

func newTestEnv(t *testing.T, s store.Store, cfg playground.Config) *testEnv {
	t.Helper()
	return newTestEnvWithClock(t, s, cfg, fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
}

// newTestEnvSQLite opens its own SQLite store and builds a testEnv. Used by
// older test files (destruction_test.go, worker_test.go) that haven't been
// converted to the per-dialect harness loop. Each call gets a fresh
// :memory: database — there is no shared state between tests.
func newTestEnvSQLite(t *testing.T, cfg playground.Config) *testEnv {
	t.Helper()
	s := stores(t)[0].Open(t) // SQLite is always the first harness
	return newTestEnv(t, s, cfg)
}

// newTestEnvWithTokens is like newTestEnvWithClock but accepts an explicit
// tokens.Service, allowing tests to inject a stub that fails at a specific
// point in the CreatePlaygroundSession 3-step sequence.
func newTestEnvWithTokens(t *testing.T, s store.Store, cfg playground.Config, svc tokens.Service) *testEnv {
	t.Helper()
	return newTestEnvWithClockAndTokens(t, s, cfg, fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)}, svc)
}

func newTestEnvWithClock(t *testing.T, s store.Store, cfg playground.Config, clk playground.Clock) *testEnv {
	t.Helper()
	svc := tokens.New(s)
	return newTestEnvWithClockAndTokens(t, s, cfg, clk, svc)
}

func newTestEnvWithClockAndTokens(t *testing.T, s store.Store, cfg playground.Config, clk playground.Clock, svc tokens.Service) *testEnv {
	t.Helper()
	stor := newStubStorage()

	// Provision the reserved playground org row so FK constraints pass.
	if err := playground.ProvisionReservedOrg(context.Background(), s, clk.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("provision reserved org: %v", err)
	}

	h := &playground.Handler{
		Store:   s,
		Tokens:  svc,
		Storage: stor,
		Cfg:     cfg,
		Clock:   clk,
		Logger:  noopLogger(),
	}

	strictAPI := openapi.NewStrictHandlerWithOptions(&playgroundOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictAPI,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	r := chi.NewRouter()
	// Unauthenticated routes.
	r.Post("/api/playground/sessions", apiWrapper.CreatePlaygroundSession)
	r.Post("/api/playground/sessions/{id}/join", apiWrapper.JoinPlaygroundSession)
	r.Get("/api/playground/sessions/{id}/tombstone", apiWrapper.GetPlaygroundTombstone)
	// Bearer-authenticated route.
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Get("/api/playground/sessions/{id}", apiWrapper.GetPlaygroundSession)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, s: s, svc: svc, stor: stor, clock: clk}
}

func defaultCfg() playground.Config {
	return playground.Config{
		Enabled:         true,
		IdleTimeout:     30 * time.Minute,
		HardCap:         24 * time.Hour,
		MaxParticipants: 5,
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func postJSON(t *testing.T, srv *httptest.Server, path, bearer string, body any) *http.Response {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func getRequest(t *testing.T, srv *httptest.Server, path, bearer string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// noopLogger returns a slog.Logger that discards all output.
func noopLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// ---------------------------------------------------------------------------
// Tests: CreatePlaygroundSession
// ---------------------------------------------------------------------------

func TestCreatePlaygroundSession_Disabled_Returns503(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.Enabled = false
			env := newTestEnv(t, h.Open(t), cfg)

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("want 503, got %d", resp.StatusCode)
			}
			var body openapi.ErrorEnvelope
			decodeJSON(t, resp, &body)
			if body.Error != "playground.disabled" {
				t.Errorf("want error=playground.disabled, got %q", body.Error)
			}
		})
	}
}

func TestCreatePlaygroundSession_EmptyBody_DefaultsApplied(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode != http.StatusCreated {
				t.Errorf("want 201, got %d", resp.StatusCode)
			}
			var body openapi.PlaygroundSessionCreated
			decodeJSON(t, resp, &body)

			if body.Bearer == "" {
				t.Error("want non-empty bearer")
			}
			if body.Nickname == "" {
				t.Error("want non-empty nickname")
			}
			if body.ExpiresAt.IsZero() {
				t.Error("want non-zero expires_at")
			}
			if body.Session.Id == "" {
				t.Error("want non-empty session id")
			}
			if body.Session.OrgId != playground.ReservedOrgID {
				t.Errorf("want org_id=%s, got %s", playground.ReservedOrgID, body.Session.OrgId)
			}
			if body.Session.Status != "active" {
				t.Errorf("want status=active, got %s", body.Session.Status)
			}
			if body.Session.MembersCount != 1 {
				t.Errorf("want members_count=1, got %d", body.Session.MembersCount)
			}
			// Default name should start with "playground-".
			if len(body.Session.Name) < len("playground-") {
				t.Errorf("want name to start with playground-, got %q", body.Session.Name)
			}
			// Default scope should be ["**"].
			if body.Session.Scope != `["**"]` {
				t.Errorf("want scope=[\"**\"], got %q", body.Session.Scope)
			}
		})
	}
}

func TestCreatePlaygroundSession_WithName_UsesProvidedName(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", map[string]string{
				"name": "my-custom-session",
				"goal": "test goal",
			})
			if resp.StatusCode != http.StatusCreated {
				t.Errorf("want 201, got %d", resp.StatusCode)
			}
			var body openapi.PlaygroundSessionCreated
			decodeJSON(t, resp, &body)
			if body.Session.Name != "my-custom-session" {
				t.Errorf("want name=my-custom-session, got %q", body.Session.Name)
			}
			if body.Session.Goal != "test goal" {
				t.Errorf("want goal=test goal, got %q", body.Session.Goal)
			}
		})
	}
}

func TestCreatePlaygroundSession_BearerIsReusable(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create session: want 201, got %d", resp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, resp, &created)

			// The bearer should work for GET /api/playground/sessions/{id}.
			getResp := getRequest(t, env.srv, "/api/playground/sessions/"+created.Session.Id, created.Bearer)
			if getResp.StatusCode != http.StatusOK {
				t.Errorf("get session with bearer: want 200, got %d", getResp.StatusCode)
			}
		})
	}
}

func TestCreatePlaygroundSession_RepoCreated(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("want 201, got %d", resp.StatusCode)
			}
			var body openapi.PlaygroundSessionCreated
			decodeJSON(t, resp, &body)

			key := playground.ReservedOrgID + "/" + body.Session.Id
			if !env.stor.repos[key] {
				t.Errorf("bare repo not created for key %s", key)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: JoinPlaygroundSession
// ---------------------------------------------------------------------------

func TestJoinPlaygroundSession_Disabled_Returns503(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.Enabled = false
			env := newTestEnv(t, h.Open(t), cfg)

			resp := postJSON(t, env.srv, "/api/playground/sessions/nonexistent/join", "", nil)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("want 503, got %d", resp.StatusCode)
			}
		})
	}
}

func TestJoinPlaygroundSession_NotFound_Returns404(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := postJSON(t, env.srv, "/api/playground/sessions/no-such-id/join", "", nil)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("want 404, got %d", resp.StatusCode)
			}
		})
	}
}

func TestJoinPlaygroundSession_Success(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			// Create a session first.
			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			// Join it.
			joinResp := postJSON(t, env.srv, "/api/playground/sessions/"+created.Session.Id+"/join", "", nil)
			if joinResp.StatusCode != http.StatusOK {
				t.Errorf("join: want 200, got %d", joinResp.StatusCode)
			}
			var joined openapi.PlaygroundJoinResult
			decodeJSON(t, joinResp, &joined)

			if joined.Bearer == "" {
				t.Error("want non-empty bearer")
			}
			if joined.Nickname == "" {
				t.Error("want non-empty nickname")
			}
			// The joiner should get a different nickname to the creator.
			if joined.Nickname == created.Nickname {
				// Not necessarily wrong (very unlikely with wordlist) but log it.
				t.Logf("note: creator and joiner got same nickname %q (very unlikely)", joined.Nickname)
			}
			if joined.Session.MembersCount != 2 {
				t.Errorf("want members_count=2, got %d", joined.Session.MembersCount)
			}
		})
	}
}

func TestJoinPlaygroundSession_WithNickname_UsesIt(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			// Join with a custom nickname that doesn't collide.
			joinResp := postJSON(t, env.srv, "/api/playground/sessions/"+created.Session.Id+"/join", "",
				map[string]string{"nickname": "custom-nick"})
			if joinResp.StatusCode != http.StatusOK {
				t.Errorf("join: want 200, got %d", joinResp.StatusCode)
			}
			var joined openapi.PlaygroundJoinResult
			decodeJSON(t, joinResp, &joined)
			if joined.Nickname != "custom-nick" {
				t.Errorf("want nickname=custom-nick, got %q", joined.Nickname)
			}
		})
	}
}

func TestJoinPlaygroundSession_SessionFull_Returns409(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			cfg := defaultCfg()
			cfg.MaxParticipants = 1
			env := newTestEnv(t, h.Open(t), cfg)

			// Create a session (1 member — the creator).
			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			// Attempt to join — session is already at MaxParticipants=1.
			joinResp := postJSON(t, env.srv, "/api/playground/sessions/"+created.Session.Id+"/join", "", nil)
			if joinResp.StatusCode != http.StatusConflict {
				t.Errorf("want 409, got %d", joinResp.StatusCode)
			}
			var errBody openapi.ErrorEnvelope
			decodeJSON(t, joinResp, &errBody)
			if errBody.Error != "playground.session_full" {
				t.Errorf("want error=playground.session_full, got %q", errBody.Error)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: GetPlaygroundSession
// ---------------------------------------------------------------------------

func TestGetPlaygroundSession_NoBearer_Returns401(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := getRequest(t, env.srv, "/api/playground/sessions/some-id", "")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestGetPlaygroundSession_ValidBearer_ReturnsSummary(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			getResp := getRequest(t, env.srv, "/api/playground/sessions/"+created.Session.Id, created.Bearer)
			if getResp.StatusCode != http.StatusOK {
				t.Errorf("get: want 200, got %d", getResp.StatusCode)
			}
			var summary openapi.PlaygroundSessionSummary
			decodeJSON(t, getResp, &summary)
			if summary.Id != created.Session.Id {
				t.Errorf("want id=%s, got %s", created.Session.Id, summary.Id)
			}
			if summary.Status != "active" {
				t.Errorf("want status=active, got %s", summary.Status)
			}
		})
	}
}

func TestGetPlaygroundSession_NotFound_Returns404(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			// First create a session to get a valid bearer, then try to get a different ID.
			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			getResp := getRequest(t, env.srv, "/api/playground/sessions/no-such-session", created.Bearer)
			if getResp.StatusCode != http.StatusNotFound {
				t.Errorf("want 404, got %d", getResp.StatusCode)
			}
		})
	}
}

func TestGetPlaygroundSession_BearerNotMember_Returns401(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			// Create two separate sessions; each has its own bearer.
			resp1 := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp1.StatusCode != http.StatusCreated {
				t.Fatalf("create1: want 201, got %d", resp1.StatusCode)
			}
			var sess1 openapi.PlaygroundSessionCreated
			decodeJSON(t, resp1, &sess1)

			resp2 := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp2.StatusCode != http.StatusCreated {
				t.Fatalf("create2: want 201, got %d", resp2.StatusCode)
			}
			var sess2 openapi.PlaygroundSessionCreated
			decodeJSON(t, resp2, &sess2)

			// Bearer from session 2 used to access session 1 — should be rejected.
			getResp := getRequest(t, env.srv, "/api/playground/sessions/"+sess1.Session.Id, sess2.Bearer)
			if getResp.StatusCode != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", getResp.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: GetPlaygroundTombstone
// ---------------------------------------------------------------------------

func TestGetPlaygroundTombstone_ActiveSession_Returns404(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			// Tombstone should not exist for an active session.
			tombResp := getRequest(t, env.srv, "/api/playground/sessions/"+created.Session.Id+"/tombstone", "")
			if tombResp.StatusCode != http.StatusNotFound {
				t.Errorf("want 404, got %d", tombResp.StatusCode)
			}
		})
	}
}

func TestGetPlaygroundTombstone_AfterDestruction_Returns200(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			createResp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if createResp.StatusCode != http.StatusCreated {
				t.Fatalf("create: want 201, got %d", createResp.StatusCode)
			}
			var created openapi.PlaygroundSessionCreated
			decodeJSON(t, createResp, &created)

			// Manually insert a tombstone row directly via the store.
			now := env.clock.Now()
			err := env.s.RecordTombstone(context.Background(), store.RecordTombstoneParams{
				SessionID:       created.Session.Id,
				OrgID:           playground.ReservedOrgID,
				MembersCount:    1,
				CommitsCount:    3,
				AutoMergesCount: 1,
				DurationSeconds: 3600,
				EndReason:       "manual",
				EndedAt:         now,
				ExpiresAt:       now.Add(30 * 24 * time.Hour),
			})
			if err != nil {
				t.Fatalf("RecordTombstone: %v", err)
			}

			tombResp := getRequest(t, env.srv, "/api/playground/sessions/"+created.Session.Id+"/tombstone", "")
			if tombResp.StatusCode != http.StatusOK {
				t.Errorf("want 200, got %d", tombResp.StatusCode)
			}
			var tomb openapi.PlaygroundTombstone
			decodeJSON(t, tombResp, &tomb)
			if tomb.SessionId != created.Session.Id {
				t.Errorf("want session_id=%s, got %s", created.Session.Id, tomb.SessionId)
			}
			if tomb.MembersCount != 1 {
				t.Errorf("want members_count=1, got %d", tomb.MembersCount)
			}
			if tomb.CommitsCount != 3 {
				t.Errorf("want commits_count=3, got %d", tomb.CommitsCount)
			}
			if string(tomb.EndReason) != "manual" {
				t.Errorf("want end_reason=manual, got %s", tomb.EndReason)
			}
		})
	}
}

func TestGetPlaygroundTombstone_UnknownSession_Returns404(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			resp := getRequest(t, env.srv, "/api/playground/sessions/no-such/tombstone", "")
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("want 404, got %d", resp.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// New tests: handler-test-coverage gap fixes
// ---------------------------------------------------------------------------

// TestJoinPlaygroundSession_HardCapElapsed_Returns410 covers handler.go:206-211
// (outer Before(*HardCapAt) check). For the outer branch we use a session
// pre-seeded with HardCapAt in the past.
func TestJoinPlaygroundSession_HardCapElapsed_Returns410(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)
			clk := fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)}
			env := newTestEnvWithClock(t, s, defaultCfg(), clk)

			// Pre-create a session via the store with HardCapAt in the past.
			past := clk.Now().Add(-1 * time.Hour)
			lastAct := past.Add(-30 * time.Minute)
			sessID := "sess-elapsed-001"
			_, err := s.CreateSession(ctx, store.CreateSessionParams{
				ID:                        sessID,
				OrgID:                     playground.ReservedOrgID,
				Name:                      "elapsed",
				Goal:                      "elapsed test",
				WritableScope:             `["**"]`,
				DefaultMode:               "sync",
				Status:                    "active",
				CreatedAt:                 past.Add(-2 * time.Hour),
				LastSubstantiveActivityAt: &lastAct,
				HardCapAt:                 &past,
				IdleTimeoutAt:             &past,
			})
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}

			joinResp := postJSON(t, env.srv, "/api/playground/sessions/"+sessID+"/join", "", nil)
			if joinResp.StatusCode != http.StatusGone {
				t.Errorf("want 410, got %d", joinResp.StatusCode)
			}
			var errBody openapi.ErrorEnvelope
			decodeJSON(t, joinResp, &errBody)
			if errBody.Error != "playground.session_ended" {
				t.Errorf("want error=playground.session_ended, got %q", errBody.Error)
			}
		})
	}
}

// TestJoinPlaygroundSession_StatusNotActive_Returns410 covers handler.go:214-219
// (the Status != "active" branch). Pre-seed a session with Status="ended" and
// HardCapAt in the future so the outer hard-cap check passes and we hit this
// branch instead.
func TestJoinPlaygroundSession_StatusNotActive_Returns410(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)
			clk := fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)}
			env := newTestEnvWithClock(t, s, defaultCfg(), clk)

			future := clk.Now().Add(1 * time.Hour)
			lastAct := clk.Now()
			sessID := "sess-ended-001"
			_, err := s.CreateSession(ctx, store.CreateSessionParams{
				ID:                        sessID,
				OrgID:                     playground.ReservedOrgID,
				Name:                      "ended",
				Goal:                      "ended test",
				WritableScope:             `["**"]`,
				DefaultMode:               "sync",
				Status:                    "ended",
				CreatedAt:                 clk.Now().Add(-1 * time.Hour),
				LastSubstantiveActivityAt: &lastAct,
				HardCapAt:                 &future,
				IdleTimeoutAt:             &future,
			})
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}

			joinResp := postJSON(t, env.srv, "/api/playground/sessions/"+sessID+"/join", "", nil)
			if joinResp.StatusCode != http.StatusGone {
				t.Errorf("want 410, got %d", joinResp.StatusCode)
			}
			var errBody openapi.ErrorEnvelope
			decodeJSON(t, joinResp, &errBody)
			if errBody.Error != "playground.session_ended" {
				t.Errorf("want error=playground.session_ended, got %q", errBody.Error)
			}
		})
	}
}

// TestCreatePlaygroundSession_InvalidScope_Returns400 covers the front-door
// scope validation. Mirrors the durable-session
// scope_validation_test.go::TestCreateSession_InvalidScope_Returns400 — same
// error code, same envelope shape — so clients can use a single branch for
// both surfaces.
func TestCreatePlaygroundSession_InvalidScope_Returns400(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			env := newTestEnv(t, h.Open(t), defaultCfg())

			// Malformed glob — same payload used in the sessions test for
			// identical-input/identical-answer guarantee.
			resp := postJSON(t, env.srv, "/api/playground/sessions", "", map[string]string{
				"scope": `["docs/{"]`,
			})
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("want 400, got %d", resp.StatusCode)
			}
			var body openapi.ErrorEnvelope
			decodeJSON(t, resp, &body)
			if body.Error != "session.invalid_writable_scope" {
				t.Errorf("want error=session.invalid_writable_scope, got %q", body.Error)
			}
			if body.Message == "" {
				t.Error("want non-empty message")
			}

			// Non-JSON payload — also rejected at the same gate.
			resp2 := postJSON(t, env.srv, "/api/playground/sessions", "", map[string]string{
				"scope": "not json",
			})
			if resp2.StatusCode != http.StatusBadRequest {
				t.Errorf("non-json: want 400, got %d", resp2.StatusCode)
			}
			var body2 openapi.ErrorEnvelope
			decodeJSON(t, resp2, &body2)
			if body2.Error != "session.invalid_writable_scope" {
				t.Errorf("non-json: want error=session.invalid_writable_scope, got %q", body2.Error)
			}
		})
	}
}

// TestCreatePlaygroundSession_RepoCreateFails_ReturnsError covers the bare-repo
// CreateRepo failure path. The session insert + bearer issue + member-add all
// succeed; CreateRepo fails. Verify the handler returns an error AND that the
// session row plus creator member row remain in the store for the destruction
// sweep to clean up later.
func TestCreatePlaygroundSession_RepoCreateFails_ReturnsError(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			s := h.Open(t)
			env := newTestEnv(t, s, defaultCfg())
			env.stor.createError = errors.New("synthetic disk-full failure")

			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode < 500 {
				t.Errorf("want 5xx error response, got %d", resp.StatusCode)
			}

			// Despite the CreateRepo failure, the session row (inserted before
			// CreateRepo) should still exist. Same for the creator member row
			// (added between bearer-issue and CreateRepo). The destruction
			// sweep cleans these by session_id.
			ctx := context.Background()
			sessions, err := s.ListExpiredPlaygroundSessions(ctx, store.ListExpiredPlaygroundSessionsParams{
				OrgID: playground.ReservedOrgID,
				Now:   time.Now().Add(48 * time.Hour), // far-future to include the new session
			})
			if err != nil {
				t.Fatalf("ListExpiredPlaygroundSessions: %v", err)
			}
			if len(sessions) == 0 {
				t.Error("expected orphaned session row to remain after CreateRepo failure, got none")
			}
		})
	}
}

// TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered covers the
// partial-failure window at handler.go:150-156: the session TX commits (step 1
// succeeds) but IssueAnonymousSessionBearer returns an error (step 2 fails),
// so step 3 (AddSessionMember) is never reached. The session row is left as an
// orphan with zero members and no bearer — the destruction sweep must clean it.
//
// Flow:
//
//  1. Arm failingTokensService so IssueAnonymousSessionBearer errors.
//  2. POST CreatePlaygroundSession — handler returns an error (5xx).
//  3. Assert: session row persists in the store (orphan present).
//  4. Assert: creator member count is 0 (step 3 never executed).
//  5. Run Destruction.Destroy on the orphaned session.
//  6. Assert: session row gone, no anon accounts remain.
func TestCreatePlaygroundSession_BearerIssuanceFails_OrphanRecovered(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)

			// Build a failingTokensService that errors on IssueAnonymousSessionBearer
			// but also captures the sessionID the handler generated (step 1 committed
			// it to the DB before calling step 2).
			realSvc := tokens.New(s)
			fts := &failingTokensService{
				real:     realSvc,
				issueErr: errors.New("synthetic bearer-issuance failure"),
			}

			env := newTestEnvWithTokens(t, s, defaultCfg(), fts)

			// Step 1+2: trigger the failure path.
			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode < 500 {
				t.Fatalf("want 5xx after bearer-issuance failure, got %d", resp.StatusCode)
			}

			// The handler must have called IssueAnonymousSessionBearer with a
			// generated session ID — capture it via the spy field.
			capturedID := fts.lastSessionID
			if capturedID == "" {
				t.Fatal("failingTokensService.lastSessionID is empty — IssueAnonymousSessionBearer was never called")
			}

			// Step 3: assert the orphaned session row exists.
			sess, err := s.GetSession(ctx, playground.ReservedOrgID, capturedID)
			if err != nil {
				t.Fatalf("session row should persist after bearer-issuance failure; GetSession: %v", err)
			}

			// Step 4: assert zero members — step 3 (AddSessionMember) was never reached.
			memberCount, err := s.CountSessionMembers(ctx, store.CountSessionMembersParams{
				OrgID:     playground.ReservedOrgID,
				SessionID: capturedID,
			})
			if err != nil {
				t.Fatalf("CountSessionMembers: %v", err)
			}
			if memberCount != 0 {
				t.Errorf("want 0 session members (step 3 skipped), got %d", memberCount)
			}

			// Bare repo must NOT exist — CreateRepo (step after bearer) was never
			// attempted because the handler returned early on step-2 failure.
			repoExists, err := env.stor.RepoExists(playground.ReservedOrgID, capturedID)
			if err != nil {
				t.Fatalf("RepoExists: %v", err)
			}
			if repoExists {
				t.Error("bare repo should not exist after a step-2 (bearer) failure")
			}

			// Step 5: run the destruction cascade directly on the orphaned session.
			d := &playground.Destruction{
				Store:        s,
				Storage:      env.stor,
				Clock:        env.clock,
				Logger:       noopLogger(),
				TombstoneTTL: 30 * 24 * time.Hour,
			}
			if err := d.Destroy(ctx, sess, "orphan_recovery"); err != nil {
				t.Fatalf("Destroy: %v", err)
			}

			// Step 6: assert full cleanup.

			// Session row must be gone.
			_, err = s.GetSession(ctx, playground.ReservedOrgID, capturedID)
			if !errors.Is(err, store.ErrNotFound) {
				t.Errorf("session row: want ErrNotFound after Destroy, got %v", err)
			}

			// No anon accounts should have been created (IssueAnonymousSessionBearer
			// failed before creating any), so ListAnonymousSessionMemberIDs should
			// return empty — but if the session-row cascade happened and the anon
			// account list was empty, Destroy should have skipped step 7 gracefully.
			// Assert member count is still 0 post-destroy (FK cascade).
			postCount, err := s.CountSessionMembers(ctx, store.CountSessionMembersParams{
				OrgID:     playground.ReservedOrgID,
				SessionID: capturedID,
			})
			if err != nil {
				t.Fatalf("CountSessionMembers after Destroy: %v", err)
			}
			if postCount != 0 {
				t.Errorf("member rows: want 0 after Destroy, got %d", postCount)
			}
		})
	}
}

// TestCreatePlaygroundSession_RepoCreateFails_DestructionSweepCleansUp is an
// end-to-end seam test for the orphan-cleanup path described in the playground
// lifecycle spec.
//
// Flow:
//
//  1. CreatePlaygroundSession with stubStorage.createError set — session row +
//     creator member row are inserted, but CreateRepo fails → 5xx.
//  2. Verify orphan state: session row present, creator member present, no bare
//     repo on disk.
//  3. Call destruction.Destroy directly (bypassing the worker sweep), passing
//     the orphaned session row.
//  4. Assert: session row gone, member row gone (FK cascade), anonymous account
//     deleted, bare repo absent (RemoveRepo is a no-op on a never-created repo).
func TestCreatePlaygroundSession_RepoCreateFails_DestructionSweepCleansUp(t *testing.T) {
	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			ctx := context.Background()
			s := h.Open(t)
			env := newTestEnv(t, s, defaultCfg())

			// Step 1: trigger the CreateRepo failure path.
			env.stor.createError = errors.New("disk full")
			resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
			if resp.StatusCode < 500 {
				t.Fatalf("want 5xx after repo-create failure, got %d", resp.StatusCode)
			}

			// Step 2: verify the orphan state.
			// The worker sweep uses ListExpiredPlaygroundSessions with a
			// far-future Now to include sessions whose hard_cap_at has not
			// yet elapsed; we do the same here to locate the orphaned row.
			farFuture := time.Now().Add(48 * time.Hour)
			orphans, err := s.ListExpiredPlaygroundSessions(ctx, store.ListExpiredPlaygroundSessionsParams{
				OrgID: playground.ReservedOrgID,
				Now:   farFuture,
			})
			if err != nil {
				t.Fatalf("ListExpiredPlaygroundSessions: %v", err)
			}
			if len(orphans) == 0 {
				t.Fatal("orphan session row should exist after CreateRepo failure, got none")
			}
			sess := orphans[0]

			// Creator member row must be present.
			memberCount, err := s.CountSessionMembers(ctx, store.CountSessionMembersParams{
				OrgID:     playground.ReservedOrgID,
				SessionID: sess.ID,
			})
			if err != nil {
				t.Fatalf("CountSessionMembers: %v", err)
			}
			if memberCount == 0 {
				t.Fatal("creator member row should exist after CreateRepo failure, got 0")
			}

			// Collect anon account IDs before destruction so we can verify deletion.
			anonIDs, err := s.ListAnonymousSessionMemberIDs(ctx, playground.ReservedOrgID, sess.ID)
			if err != nil {
				t.Fatalf("ListAnonymousSessionMemberIDs: %v", err)
			}
			if len(anonIDs) == 0 {
				t.Fatal("expected at least one anonymous member account, got none")
			}

			// Bare repo should NOT exist (CreateRepo was never called successfully).
			repoExists, err := env.stor.RepoExists(playground.ReservedOrgID, sess.ID)
			if err != nil {
				t.Fatalf("RepoExists: %v", err)
			}
			if repoExists {
				t.Error("bare repo should not exist after a failed CreateRepo")
			}

			// Step 3: run the destruction cascade directly.
			// Clear createError so the storage stub doesn't interfere with
			// RemoveRepo (which is a delete — not gated by createError anyway,
			// but belt-and-suspenders).
			env.stor.createError = nil
			d := &playground.Destruction{
				Store:        s,
				Storage:      env.stor,
				Clock:        env.clock,
				Logger:       noopLogger(),
				TombstoneTTL: 30 * 24 * time.Hour,
			}
			if err := d.Destroy(ctx, sess, "manual"); err != nil {
				t.Fatalf("Destroy: %v", err)
			}

			// Step 4: assert full cleanup.

			// Session row must be gone.
			_, err = s.GetSession(ctx, playground.ReservedOrgID, sess.ID)
			if !errors.Is(err, store.ErrNotFound) {
				t.Errorf("session row: want ErrNotFound after Destroy, got %v", err)
			}

			// Member row must be gone (FK cascade from session delete).
			postCount, err := s.CountSessionMembers(ctx, store.CountSessionMembersParams{
				OrgID:     playground.ReservedOrgID,
				SessionID: sess.ID,
			})
			if err != nil {
				t.Fatalf("CountSessionMembers after Destroy: %v", err)
			}
			if postCount != 0 {
				t.Errorf("member rows: want 0 after Destroy, got %d", postCount)
			}

			// Anonymous account(s) must be deleted (not cascaded — deleted explicitly
			// by Destruction step 7).
			for _, anonID := range anonIDs {
				_, err := s.GetAccountByID(ctx, anonID)
				if !errors.Is(err, store.ErrNotFound) {
					t.Errorf("anon account %s: want ErrNotFound after Destroy, got %v", anonID, err)
				}
			}

			// Bare repo remains absent (RemoveRepo on a non-existent key is a
			// no-op in stubStorage — this exercises the idempotency of step 8).
			repoExists, err = env.stor.RepoExists(playground.ReservedOrgID, sess.ID)
			if err != nil {
				t.Fatalf("RepoExists after Destroy: %v", err)
			}
			if repoExists {
				t.Error("bare repo: should remain absent after Destroy of an orphan with no repo")
			}
		})
	}
}
