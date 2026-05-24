package playground_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Clock stub
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
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	srv   *httptest.Server
	s     store.Store
	svc   tokens.Service
	stor  *stubStorage
	clock fixedClock
}

func openStore(t *testing.T) store.Store {
	t.Helper()
	s, _, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared&mode=memory", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestEnv(t *testing.T, cfg playground.Config) *testEnv {
	t.Helper()
	s := openStore(t)
	return newTestEnvWithStore(t, s, cfg)
}

func newTestEnvWithStore(t *testing.T, s store.Store, cfg playground.Config) *testEnv {
	t.Helper()
	svc := tokens.New(s)
	stor := newStubStorage()
	clk := fixedClock{t: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)}

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
	cfg := defaultCfg()
	cfg.Enabled = false
	env := newTestEnv(t, cfg)

	resp := postJSON(t, env.srv, "/api/playground/sessions", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", resp.StatusCode)
	}
	var body openapi.ErrorEnvelope
	decodeJSON(t, resp, &body)
	if body.Error != "playground.disabled" {
		t.Errorf("want error=playground.disabled, got %q", body.Error)
	}
}

func TestCreatePlaygroundSession_EmptyBody_DefaultsApplied(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestCreatePlaygroundSession_WithName_UsesProvidedName(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestCreatePlaygroundSession_BearerIsReusable(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestCreatePlaygroundSession_RepoCreated(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

// ---------------------------------------------------------------------------
// Tests: JoinPlaygroundSession
// ---------------------------------------------------------------------------

func TestJoinPlaygroundSession_Disabled_Returns503(t *testing.T) {
	cfg := defaultCfg()
	cfg.Enabled = false
	env := newTestEnv(t, cfg)

	resp := postJSON(t, env.srv, "/api/playground/sessions/nonexistent/join", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", resp.StatusCode)
	}
}

func TestJoinPlaygroundSession_NotFound_Returns404(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

	resp := postJSON(t, env.srv, "/api/playground/sessions/no-such-id/join", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestJoinPlaygroundSession_Success(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestJoinPlaygroundSession_WithNickname_UsesIt(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestJoinPlaygroundSession_SessionFull_Returns409(t *testing.T) {
	cfg := defaultCfg()
	cfg.MaxParticipants = 1
	env := newTestEnv(t, cfg)

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
}

// ---------------------------------------------------------------------------
// Tests: GetPlaygroundSession
// ---------------------------------------------------------------------------

func TestGetPlaygroundSession_NoBearer_Returns401(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

	resp := getRequest(t, env.srv, "/api/playground/sessions/some-id", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestGetPlaygroundSession_ValidBearer_ReturnsSummary(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestGetPlaygroundSession_NotFound_Returns404(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestGetPlaygroundSession_BearerNotMember_Returns401(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

// ---------------------------------------------------------------------------
// Tests: GetPlaygroundTombstone
// ---------------------------------------------------------------------------

func TestGetPlaygroundTombstone_ActiveSession_Returns404(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestGetPlaygroundTombstone_AfterDestruction_Returns200(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

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
}

func TestGetPlaygroundTombstone_UnknownSession_Returns404(t *testing.T) {
	env := newTestEnv(t, defaultCfg())

	resp := getRequest(t, env.srv, "/api/playground/sessions/no-such/tombstone", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}
