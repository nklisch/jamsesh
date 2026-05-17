package sessions_test

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
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Sender stub
// ---------------------------------------------------------------------------

// stubSender records sent emails for assertions in tests.
type stubSender struct {
	sent []stubEmail
}

type stubEmail struct {
	recipient, subject, body string
}

func (s *stubSender) Send(_ context.Context, recipient, subject, body string) error {
	s.sent = append(s.sent, stubEmail{recipient, subject, body})
	return nil
}

// ---------------------------------------------------------------------------
// Storage stub
// ---------------------------------------------------------------------------

// stubStorage is an in-memory storage.Service for tests. It records repo
// creates and can be configured to fail.
type stubStorage struct {
	repos       map[string]bool
	createError error // injected failure on CreateRepo
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
// StrictServerInterface shim — stubs out methods not under test
// ---------------------------------------------------------------------------

type sessionsOnlyStrict struct {
	*sessions.Handler
}

func (h *sessionsOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("not wired")
}
func (h *sessionsOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("not wired")
}

var _ openapi.StrictServerInterface = (*sessionsOnlyStrict)(nil)

// ---------------------------------------------------------------------------
// HTTP helpers (extended)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	srv     *httptest.Server
	svc     tokens.Service
	s       store.Store
	stor    *stubStorage
	eventLog *events.Log
}

func openStore(t *testing.T) store.Store {
	t.Helper()
	// Use a unique file path per test to avoid shared-cache interference.
	s, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s := openStore(t)
	svc := tokens.New(s)
	stor := newStubStorage()
	log := events.New(s)
	h := sessions.New(s, stor, log, &stubSender{}, "http://localhost:8443")
	strictAPI := openapi.NewStrictHandler(&sessionsOnlyStrict{h}, nil)
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler: strictAPI,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		},
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Get("/api/orgs/{orgID}/sessions", apiWrapper.ListSessions)
		r.Post("/api/orgs/{orgID}/sessions", apiWrapper.CreateSession)
		r.Get("/api/orgs/{orgID}/sessions/{sessionID}", apiWrapper.GetSession)
		r.Patch("/api/orgs/{orgID}/sessions/{sessionID}", apiWrapper.PatchSession)
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/finalize", apiWrapper.FinalizeSession)
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/abandon", apiWrapper.AbandonSession)
		r.Get("/api/orgs/{orgID}/sessions/{sessionID}/refs", apiWrapper.ListSessionRefs)
		r.Get("/api/orgs/{orgID}/sessions/{sessionID}/digest", apiWrapper.GetSessionDigest)
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/invites", apiWrapper.InviteToSession)
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept", apiWrapper.AcceptSessionInvite)
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove", apiWrapper.RemoveSessionMember)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, svc: svc, s: s, stor: stor, eventLog: log}
}

func (e *testEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

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
	t.Cleanup(func() { _ = resp.Body.Close() })
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
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

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

func seedOrgMember(t *testing.T, s store.Store, orgID, accountID, role string) {
	t.Helper()
	if err := s.AddOrgMember(context.Background(), store.AddOrgMemberParams{
		OrgID:     orgID,
		AccountID: accountID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed org member: %v", err)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions tests
// ---------------------------------------------------------------------------

func TestCreateSession_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "test-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)

	body := map[string]any{
		"name":         "Fix auth bug",
		"goal":         "Resolve auth race condition",
		"scope":        `["src/auth/**"]`,
		"default_mode": "sync",
	}

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var sess openapi.Session
	decodeBody(t, resp, &sess)

	if sess.Status != "active" {
		t.Errorf("expected status=active, got %q", sess.Status)
	}
	if sess.Goal != "Resolve auth race condition" {
		t.Errorf("unexpected goal: %q", sess.Goal)
	}
	if len(sess.Members) != 1 {
		t.Errorf("expected 1 member, got %d", len(sess.Members))
	}
	if sess.Members[0].Role != "creator" {
		t.Errorf("expected creator role, got %q", sess.Members[0].Role)
	}

	// Verify the bare repo was created.
	exists, _ := env.stor.RepoExists(org.ID, sess.Id)
	if !exists {
		t.Error("bare repo was not created")
	}
}

func TestCreateSession_RepoFailureRollsBack(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "test-org-rb")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	// Inject a storage failure.
	env.stor.createError = errors.New("disk full")

	token := env.bearerToken(t, acc.ID)
	body := map[string]any{
		"name":         "Fail Session",
		"goal":         "Will fail on repo",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token, body)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 on repo failure, got %d", resp.StatusCode)
	}

	// Verify the session row was cleaned up.
	all, err := env.s.ListSessionsForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected session to be cleaned up after repo failure, but %d row(s) remain", len(all))
	}
}

func TestCreateSession_NotOrgMember_Returns403(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "outsider@example.com")
	org := seedOrg(t, env.s, "Test Org", "test-org-403")

	token := env.bearerToken(t, acc.ID)
	body := map[string]any{
		"name":         "Not My Org",
		"goal":         "Should fail",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token, body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// PATCH /api/orgs/{orgID}/sessions/{sessionID} tests
// ---------------------------------------------------------------------------

func createSession(t *testing.T, env *testEnv, orgID, accID string) openapi.Session {
	t.Helper()
	token := env.bearerToken(t, accID)
	body := map[string]any{
		"name":         "Test Session",
		"goal":         "Initial goal",
		"scope":        `["src/**"]`,
		"default_mode": "sync",
	}
	resp := postJSON(t, env.srv, "/api/orgs/"+orgID+"/sessions", token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createSession: expected 201, got %d", resp.StatusCode)
	}
	var sess openapi.Session
	decodeBody(t, resp, &sess)
	return sess
}

func TestPatchSession_WideScope(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "patch-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	path := "/api/orgs/" + org.ID + "/sessions/" + sess.Id
	body := map[string]any{
		"scope": `["src/**","tests/**"]`,
		"goal":  "Updated goal",
	}
	resp := patchJSON(t, env.srv, path, token, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated openapi.Session
	decodeBody(t, resp, &updated)
	if updated.Goal != "Updated goal" {
		t.Errorf("expected updated goal, got %q", updated.Goal)
	}
}

func TestPatchSession_ScopeNarrowingRejected(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "narrow-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	path := "/api/orgs/" + org.ID + "/sessions/" + sess.Id
	body := map[string]any{
		// Only "tests/**" — removes "src/**" which was in original scope.
		"scope": `["tests/**"]`,
	}
	resp := patchJSON(t, env.srv, path, token, body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errEnv openapi.ErrorEnvelope
	decodeBody(t, resp, &errEnv)
	if errEnv.Error != "session.scope_narrowing_rejected" {
		t.Errorf("expected scope_narrowing_rejected, got %q", errEnv.Error)
	}
}

func TestPatchSession_NonCreatorForbidden(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	member := seedAccount(t, env.s, "member@example.com")
	org := seedOrg(t, env.s, "Test Org", "perm-org")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, member.ID, "member")

	sess := createSession(t, env, org.ID, creator.ID)
	memberToken := env.bearerToken(t, member.ID)

	// Add member to session so the GetSessionMember check finds them but wrong role.
	_ = env.s.AddSessionMember(context.Background(), store.AddSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sess.Id,
		AccountID: member.ID,
		Role:      "member",
		JoinedAt:  time.Now().UTC(),
	})

	path := "/api/orgs/" + org.ID + "/sessions/" + sess.Id
	body := map[string]any{"goal": "Hijack goal"}
	resp := patchJSON(t, env.srv, path, memberToken, body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/finalize tests
// ---------------------------------------------------------------------------

func TestFinalizeSession_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "fin-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/finalize", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated openapi.Session
	decodeBody(t, resp, &updated)
	if updated.Status != "finalizing" {
		t.Errorf("expected status=finalizing, got %q", updated.Status)
	}
}

func TestFinalizeSession_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "fin-idem-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	// First finalize.
	resp1 := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/finalize", token, nil)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first finalize: expected 200, got %d", resp1.StatusCode)
	}

	// Second finalize — should be idempotent 200, no duplicate event.
	resp2 := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/finalize", token, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second finalize: expected 200, got %d", resp2.StatusCode)
	}

	// Verify only one session.finalizing event was emitted.
	evts, err := env.s.ListEventsSince(context.Background(), store.ListEventsSinceParams{
		SessionID: sess.Id,
		SinceSeq:  -1,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var finalizingCount int
	for _, e := range evts {
		if e.Type == "session.finalizing" {
			finalizingCount++
		}
	}
	if finalizingCount != 1 {
		t.Errorf("expected exactly 1 session.finalizing event, got %d", finalizingCount)
	}
}

func TestFinalizeSession_EndedReturns409(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "fin-ended-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	// Abandon first to put it in ended state.
	_ = postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/abandon", token, nil)

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/finalize", token, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// POST /api/orgs/{orgID}/sessions/{sessionID}/abandon tests
// ---------------------------------------------------------------------------

func TestAbandonSession_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "abn-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/abandon", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated openapi.Session
	decodeBody(t, resp, &updated)
	if updated.Status != "ended" {
		t.Errorf("expected status=ended, got %q", updated.Status)
	}
	if updated.EndReason != "abandoned" {
		t.Errorf("expected end_reason=abandoned, got %q", updated.EndReason)
	}

	// Verify session.ended event was emitted.
	evts, _ := env.s.ListEventsSince(context.Background(), store.ListEventsSinceParams{
		SessionID: sess.Id,
		SinceSeq:  -1,
		Limit:     100,
	})
	var endedCount int
	for _, e := range evts {
		if e.Type == "session.ended" {
			endedCount++
		}
	}
	if endedCount != 1 {
		t.Errorf("expected 1 session.ended event, got %d", endedCount)
	}
}

func TestAbandonSession_DoubleFireNoDoubleEvent(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator@example.com")
	org := seedOrg(t, env.s, "Test Org", "abn-dbl-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	sess := createSession(t, env, org.ID, acc.ID)
	token := env.bearerToken(t, acc.ID)

	// First abandon.
	resp1 := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/abandon", token, nil)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first abandon: expected 200, got %d", resp1.StatusCode)
	}

	// Second abandon — already ended, should return 409.
	resp2 := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/abandon", token, nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("second abandon: expected 409, got %d", resp2.StatusCode)
	}

	// Verify only one session.ended event was emitted.
	evts, _ := env.s.ListEventsSince(context.Background(), store.ListEventsSinceParams{
		SessionID: sess.Id,
		SinceSeq:  -1,
		Limit:     100,
	})
	var endedCount int
	for _, e := range evts {
		if e.Type == "session.ended" {
			endedCount++
		}
	}
	if endedCount != 1 {
		t.Errorf("expected exactly 1 session.ended event, got %d", endedCount)
	}
}

func TestAbandonSession_NonCreatorForbidden(t *testing.T) {
	env := newTestEnv(t)
	creator := seedAccount(t, env.s, "creator@example.com")
	member := seedAccount(t, env.s, "member@example.com")
	org := seedOrg(t, env.s, "Test Org", "abn-perm-org")
	seedOrgMember(t, env.s, org.ID, creator.ID, "creator")
	seedOrgMember(t, env.s, org.ID, member.ID, "member")

	sess := createSession(t, env, org.ID, creator.ID)
	memberToken := env.bearerToken(t, member.ID)

	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions/"+sess.Id+"/abandon", memberToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
