package sessions_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/tokens"
)

// filesTestEnv wires only the GetSessionFile endpoint so we can test it
// independently. Shares the sessionsOnlyStrict shim from handler_test.go.
type filesTestEnv struct {
	srv    *httptest.Server
	svc    tokens.Service
	s      store.Store
	stor   *stubStorage
	log    *events.Log
}

// buildFilesEnv is the same as newTestEnv but also registers GetSessionFile.
func buildFilesEnv(t *testing.T) *filesTestEnv {
	t.Helper()
	s, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared&mode=memory", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
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
		r.Get("/api/orgs/{orgID}/sessions/{sessionID}/files", apiWrapper.GetSessionFile)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &filesTestEnv{srv: srv, svc: svc, s: s, stor: stor, log: log}
}

func (e *filesTestEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

// ---------------------------------------------------------------------------
// Helper: create account + org + session + session member
// ---------------------------------------------------------------------------

func setupSession(t *testing.T, s store.Store) (orgID, sessionID, accountID string) {
	t.Helper()
	accountID = "acc-files-" + t.Name()
	orgID = "org-files-" + t.Name()
	sessionID = "sess-files-" + t.Name()

	now := time.Now().UTC()

	if _, err := s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          accountID,
		Email:       accountID + "@test.com",
		DisplayName: accountID,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := s.CreateOrg(context.Background(), store.CreateOrgParams{
		ID:        orgID,
		Name:      orgID,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := s.AddOrgMember(context.Background(), store.AddOrgMemberParams{
		OrgID:     orgID,
		AccountID: accountID,
		Role:      "creator",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if _, err := s.CreateSession(context.Background(), store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         orgID,
		Name:          "test session",
		Goal:          "test goal",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(context.Background(), store.AddSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: accountID,
		Role:      "creator",
		JoinedAt:  now,
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}
	return
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGetSessionFile_Unauthenticated(t *testing.T) {
	e := buildFilesEnv(t)

	resp := getRequest(t, e.srv, "/api/orgs/org1/sessions/sess1/files?commit=abc&path=foo.go", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestGetSessionFile_NotOrgMember(t *testing.T) {
	e := buildFilesEnv(t)
	// Create account but no org membership.
	now := time.Now().UTC()
	accountID := "acc-no-org-" + t.Name()
	if _, err := e.s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          accountID,
		Email:       accountID + "@test.com",
		DisplayName: accountID,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	bearer := e.bearerToken(t, accountID)
	resp := getRequest(t, e.srv, "/api/orgs/non-existent-org/sessions/sess1/files?commit=abc&path=foo.go", bearer)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestGetSessionFile_SessionNotFound(t *testing.T) {
	e := buildFilesEnv(t)
	orgID, _, accountID := setupSession(t, e.s)
	bearer := e.bearerToken(t, accountID)

	resp := getRequest(t, e.srv, "/api/orgs/"+orgID+"/sessions/nonexistent/files?commit=abc&path=foo.go", bearer)
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		// either is acceptable: session member check fails (403) or session not found (404)
		t.Errorf("want 403 or 404, got %d", resp.StatusCode)
	}
}

func TestGetSessionFile_RepoNotFound(t *testing.T) {
	e := buildFilesEnv(t)
	orgID, sessionID, accountID := setupSession(t, e.s)
	// Do NOT call CreateRepo so the repo dir doesn't exist.
	bearer := e.bearerToken(t, accountID)

	resp := getRequest(t, e.srv, "/api/orgs/"+orgID+"/sessions/"+sessionID+"/files?commit=0000000000000000000000000000000000000000&path=foo.go", bearer)
	// Stub storage returns a non-existent path, so PlainOpen fails → 500
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 (repo not found), got %d", resp.StatusCode)
	}
}
