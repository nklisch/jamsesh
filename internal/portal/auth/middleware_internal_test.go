package auth

// Internal tests for auth middleware — use package-private symbols directly.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

func openStoreInternal(t *testing.T) store.Store {
	t.Helper()
	s, _, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedAccountInternal(t *testing.T, s store.Store, email string) store.Account {
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

func seedOrgInternal(t *testing.T, s store.Store, name string) store.Org {
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

func seedMemberInternal(t *testing.T, s store.Store, orgID, accountID, role string) {
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

func issueTokenInternal(t *testing.T, svc tokens.Service, accountID string) string {
	t.Helper()
	pair, err := svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

func TestRequireOrgRole_OrgMemberInContext(t *testing.T) {
	s := openStoreInternal(t)
	svc := tokens.New(s)

	acc := seedAccountInternal(t, s, "eve@example.com")
	org := seedOrgInternal(t, s, "test-org5")
	seedMemberInternal(t, s, org.ID, acc.ID, "creator")

	var capturedMember *store.OrgMember

	r := chi.NewRouter()
	r.Route("/api/orgs/{orgID}", func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Use(RequireOrgRole(s, "creator"))
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			m, ok := orgMemberFromContext(req.Context())
			if ok {
				capturedMember = m
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	tok := issueTokenInternal(t, svc, acc.ID)

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
