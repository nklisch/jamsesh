package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// seedAccount inserts a bare account row and returns it.
func seedAccount(t *testing.T, s store.Store, email string) store.Account {
	t.Helper()
	acc, err := s.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          uuid.New().String(),
		Email:       email,
		DisplayName: email,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return acc
}

// seedOrg inserts a bare org row and returns it.
func seedOrg(t *testing.T, s store.Store, name string) store.Org {
	t.Helper()
	org, err := s.CreateOrg(context.Background(), store.CreateOrgParams{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      name,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	return org
}

// seedMember links account to org with the given role.
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

// buildMiddlewareChain builds a chi router with RequireOrgRole applied and a
// trivial next handler that records whether it was called.
func buildMiddlewareChain(s store.Store, tokenSvc tokens.Service, roles ...string) (*httptest.Server, *bool) {
	called := false

	r := chi.NewRouter()
	r.Route("/api/orgs/{orgID}", func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(tokenSvc))
		r.Use(auth.RequireOrgRole(s, roles...))
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})
	})

	srv := httptest.NewServer(r)
	return srv, &called
}

// issueToken creates a valid access token for the given account ID.
func issueToken(t *testing.T, s store.Store, svc tokens.Service, accountID string) string {
	t.Helper()
	// Ensure account exists in store so BearerMiddleware can load it.
	// (The account must already exist via seedAccount before calling this.)
	pair, err := svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRequireOrgRole_NotAMember_Returns403(t *testing.T) {
	s := openStore(t)
	svc := tokens.New(s)

	acc := seedAccount(t, s, "alice@example.com")
	org := seedOrg(t, s, "test-org")
	// alice is NOT a member of org

	srv, called := buildMiddlewareChain(s, svc, "creator", "member")
	t.Cleanup(srv.Close)

	tok := issueToken(t, s, svc, acc.ID)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orgs/"+org.ID+"/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	if *called {
		t.Error("next handler should not have been called")
	}
}

func TestRequireOrgRole_WrongRole_Returns403(t *testing.T) {
	s := openStore(t)
	svc := tokens.New(s)

	acc := seedAccount(t, s, "bob@example.com")
	org := seedOrg(t, s, "test-org2")
	seedMember(t, s, org.ID, acc.ID, "member") // bob is a member, not creator

	// Require creator role only
	srv, called := buildMiddlewareChain(s, svc, "creator")
	t.Cleanup(srv.Close)

	tok := issueToken(t, s, svc, acc.ID)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orgs/"+org.ID+"/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	if *called {
		t.Error("next handler should not have been called")
	}
}

func TestRequireOrgRole_CorrectRole_PassesThrough(t *testing.T) {
	s := openStore(t)
	svc := tokens.New(s)

	acc := seedAccount(t, s, "carol@example.com")
	org := seedOrg(t, s, "test-org3")
	seedMember(t, s, org.ID, acc.ID, "creator")

	srv, called := buildMiddlewareChain(s, svc, "creator", "member")
	t.Cleanup(srv.Close)

	tok := issueToken(t, s, svc, acc.ID)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orgs/"+org.ID+"/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !*called {
		t.Error("next handler should have been called")
	}
}

func TestRequireOrgRole_MemberRole_AllowedWhenInSet(t *testing.T) {
	s := openStore(t)
	svc := tokens.New(s)

	acc := seedAccount(t, s, "dave@example.com")
	org := seedOrg(t, s, "test-org4")
	seedMember(t, s, org.ID, acc.ID, "member")

	srv, called := buildMiddlewareChain(s, svc, "creator", "member")
	t.Cleanup(srv.Close)

	tok := issueToken(t, s, svc, acc.ID)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orgs/"+org.ID+"/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !*called {
		t.Error("next handler should have been called")
	}
}

func TestRequireOrgRole_OrgMemberInContext(t *testing.T) {
	s := openStore(t)
	svc := tokens.New(s)

	acc := seedAccount(t, s, "eve@example.com")
	org := seedOrg(t, s, "test-org5")
	seedMember(t, s, org.ID, acc.ID, "creator")

	var capturedMember *store.OrgMember

	r := chi.NewRouter()
	r.Route("/api/orgs/{orgID}", func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Use(auth.RequireOrgRole(s, "creator"))
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			m, ok := auth.OrgMemberFromContext(req.Context())
			if ok {
				capturedMember = m
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	tok := issueToken(t, s, svc, acc.ID)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/orgs/"+org.ID+"/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if capturedMember == nil {
		t.Fatal("expected OrgMember in context, got nil")
	}
	if capturedMember.Role != "creator" {
		t.Errorf("expected role creator, got %s", capturedMember.Role)
	}
}
