package sessions_test

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
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/tokens"
)

// refmodesTestEnv wires only the UpsertRefMode endpoint.
type refmodesTestEnv struct {
	srv *httptest.Server
	svc tokens.Service
	s   store.Store
	log *events.Log
}

func buildRefmodesEnv(t *testing.T) *refmodesTestEnv {
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
		r.Post("/api/orgs/{orgID}/sessions/{sessionID}/ref-modes", apiWrapper.UpsertRefMode)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &refmodesTestEnv{srv: srv, svc: svc, s: s, log: log}
}

func (e *refmodesTestEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

func postRefMode(t *testing.T, srv *httptest.Server, orgID, sessionID, bearer string, body any) *http.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/orgs/"+orgID+"/sessions/"+sessionID+"/ref-modes",
		bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST ref-modes: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestUpsertRefMode_Unauthenticated(t *testing.T) {
	e := buildRefmodesEnv(t)
	resp := postRefMode(t, e.srv, "org1", "sess1", "", map[string]any{"ref": "refs/heads/jam/s/u/main", "mode": "sync"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestUpsertRefMode_NotOrgMember(t *testing.T) {
	e := buildRefmodesEnv(t)
	orgID, sessionID, _ := setupSession(t, e.s)

	// Create an account that has no org membership.
	unrelatedID := "unrelated-" + t.Name()
	now := time.Now().UTC()
	if _, err := e.s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          unrelatedID,
		Email:       unrelatedID + "@test.com",
		DisplayName: unrelatedID,
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	bearer := e.bearerToken(t, unrelatedID)
	resp := postRefMode(t, e.srv, orgID, sessionID, bearer, map[string]any{"ref": "refs/heads/jam/s/u/main", "mode": "sync"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestUpsertRefMode_SessionNotFound(t *testing.T) {
	e := buildRefmodesEnv(t)
	orgID, _, accountID := setupSession(t, e.s)
	bearer := e.bearerToken(t, accountID)

	resp := postRefMode(t, e.srv, orgID, "nonexistent-session", bearer, map[string]any{"ref": "refs/heads/jam/s/u/main", "mode": "sync"})
	// session member lookup fails → 403
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 403 or 404, got %d", resp.StatusCode)
	}
}

func TestUpsertRefMode_Success(t *testing.T) {
	e := buildRefmodesEnv(t)
	orgID, sessionID, accountID := setupSession(t, e.s)
	bearer := e.bearerToken(t, accountID)

	ref := "refs/heads/jam/" + sessionID + "/" + accountID + "/main"
	resp := postRefMode(t, e.srv, orgID, sessionID, bearer, map[string]any{
		"ref":  ref,
		"mode": "isolated",
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("want 204, got %d", resp.StatusCode)
	}

	// Verify the ref mode was persisted.
	rm, err := e.s.GetRefMode(context.Background(), store.GetRefModeParams{
		SessionID: sessionID,
		Ref:       ref,
	})
	if err != nil {
		t.Fatalf("get ref mode: %v", err)
	}
	if rm.Mode != "isolated" {
		t.Errorf("want mode=isolated, got %q", rm.Mode)
	}
}

func TestUpsertRefMode_UpdateExisting(t *testing.T) {
	e := buildRefmodesEnv(t)
	orgID, sessionID, accountID := setupSession(t, e.s)
	bearer := e.bearerToken(t, accountID)
	ref := "refs/heads/jam/" + sessionID + "/" + accountID + "/main"

	// First upsert.
	resp1 := postRefMode(t, e.srv, orgID, sessionID, bearer, map[string]any{"ref": ref, "mode": "sync"})
	if resp1.StatusCode != http.StatusNoContent {
		t.Fatalf("first upsert: want 204, got %d", resp1.StatusCode)
	}

	// Second upsert switches mode.
	resp2 := postRefMode(t, e.srv, orgID, sessionID, bearer, map[string]any{"ref": ref, "mode": "isolated"})
	if resp2.StatusCode != http.StatusNoContent {
		t.Fatalf("second upsert: want 204, got %d", resp2.StatusCode)
	}

	rm, _ := e.s.GetRefMode(context.Background(), store.GetRefModeParams{SessionID: sessionID, Ref: ref})
	if rm.Mode != "isolated" {
		t.Errorf("want mode=isolated after update, got %q", rm.Mode)
	}
}
