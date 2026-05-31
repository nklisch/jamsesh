package store_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

// TestOpenSQLiteInMemory verifies that db.Open("sqlite", ":memory:") returns a
// working Store and that migrations have run.
func TestOpenSQLiteInMemory(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open sqlite :memory: %v", err)
	}
	defer s.Close()

	if s.Dialect() != "sqlite" {
		t.Fatalf("dialect: got %q, want %q", s.Dialect(), "sqlite")
	}

	// Basic round-trip: create an org and fetch it back.
	now := time.Now().UTC().Truncate(time.Second)
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "01HX0000000000000000000001",
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if org.ID != "01HX0000000000000000000001" {
		t.Errorf("org.ID: got %q, want %q", org.ID, "01HX0000000000000000000001")
	}

	fetched, err := s.GetOrgBySlug(ctx, "test-org")
	if err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	if fetched.Name != "Test Org" {
		t.Errorf("fetched.Name: got %q, want %q", fetched.Name, "Test Org")
	}
}

// TestUnknownDriverError verifies that db.Open returns a descriptive error for
// unknown drivers.
func TestUnknownDriverError(t *testing.T) {
	ctx := context.Background()
	_, _, err := db.Open(ctx, "mysql", "dsn", db.PoolConfig{})
	if err == nil {
		t.Fatal("expected error for unknown driver, got nil")
	}
	if !contains(err.Error(), "mysql") {
		t.Errorf("error %q does not mention driver name %q", err.Error(), "mysql")
	}
}

// TestErrNotFound verifies that a missing row returns store.ErrNotFound.
func TestErrNotFound(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	_, err = s.GetOrgByID(ctx, "nonexistent-id")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected store.ErrNotFound, got %v", err)
	}
}

// TestErrUniqueViolation verifies that a duplicate slug returns
// store.ErrUniqueViolation.
func TestErrUniqueViolation(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	p := store.CreateOrgParams{
		ID:        "01HX0000000000000000000002",
		Name:      "Org A",
		Slug:      "dup-slug",
		CreatedAt: now,
	}
	if _, err := s.CreateOrg(ctx, p); err != nil {
		t.Fatalf("first CreateOrg: %v", err)
	}

	p.ID = "01HX0000000000000000000003"
	p.Name = "Org B"
	_, err = s.CreateOrg(ctx, p)
	if !errors.Is(err, store.ErrUniqueViolation) {
		t.Fatalf("expected store.ErrUniqueViolation on duplicate slug, got %v", err)
	}
}

// TestCloseAndReopen verifies that Close() releases resources and the same
// SQLite file can be reopened.
func TestCloseAndReopen(t *testing.T) {
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s1, _, err := db.Open(ctx, "sqlite", path, db.PoolConfig{})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	now := time.Now().UTC()
	if _, err := s1.CreateOrg(ctx, store.CreateOrgParams{
		ID: "01HX0000000000000000000010", Name: "Org", Slug: "org-reopen", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, _, err := db.Open(ctx, "sqlite", path, db.PoolConfig{})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	org, err := s2.GetOrgBySlug(ctx, "org-reopen")
	if err != nil {
		t.Fatalf("GetOrgBySlug after reopen: %v", err)
	}
	if org.ID != "01HX0000000000000000000010" {
		t.Errorf("unexpected org id after reopen: %q", org.ID)
	}
}

// TestNullableGitHubUserID verifies round-trip of nil and non-nil github_user_id.
func TestNullableGitHubUserID(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()

	// Account with nil GitHub ID.
	acc1, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:           "acc-01",
		Email:        "alice@example.com",
		DisplayName:  "Alice",
		GithubUserID: nil,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("CreateAccount nil github: %v", err)
	}
	if acc1.GithubUserID != nil {
		t.Errorf("expected nil GithubUserID, got %v", acc1.GithubUserID)
	}

	// Account with non-nil GitHub ID.
	ghID := "gh-12345"
	acc2, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:           "acc-02",
		Email:        "bob@example.com",
		DisplayName:  "Bob",
		GithubUserID: &ghID,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("CreateAccount with github: %v", err)
	}
	if acc2.GithubUserID == nil || *acc2.GithubUserID != ghID {
		t.Errorf("GithubUserID: got %v, want %q", acc2.GithubUserID, ghID)
	}

	// Fetch and verify.
	fetched, err := s.GetAccountByID(ctx, "acc-02")
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if fetched.GithubUserID == nil || *fetched.GithubUserID != ghID {
		t.Errorf("fetched GithubUserID: got %v, want %q", fetched.GithubUserID, ghID)
	}
}

// TestSessionCRUD verifies CreateSession / GetSession / ListSessionsForOrg.
func TestSessionCRUD(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()

	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "org-sess", Name: "Org", Slug: "org-sess", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-01",
		OrgID:         org.ID,
		Name:          "Sprint 1",
		Goal:          "Ship it",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		BaseSHA:       nil,
		Status:        "active",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID != "sess-01" {
		t.Errorf("sess.ID: got %q, want %q", sess.ID, "sess-01")
	}

	got, err := s.GetSession(ctx, org.ID, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("status: got %q, want %q", got.Status, "active")
	}

	list, err := s.ListSessionsForOrg(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListSessionsForOrg: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len: got %d, want 1", len(list))
	}
}

// TestMagicLinkSingleUse verifies that ConsumeMagicLinkToken is a no-op on
// the second call (used_at IS NULL guard).
func TestMagicLinkSingleUse(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	token, err := s.CreateMagicLinkToken(ctx, store.CreateMagicLinkTokenParams{
		ID:        "tok-01",
		TokenHash: "hash-abc",
		Email:     "user@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateMagicLinkToken: %v", err)
	}

	usedAt := now.Add(time.Minute)
	if _, err := s.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{
		ID:     token.ID,
		UsedAt: &usedAt,
	}); err != nil {
		t.Fatalf("first ConsumeMagicLinkToken: %v", err)
	}

	// Second consume must not error (WHERE used_at IS NULL matches 0 rows → 0 affected).
	if _, err := s.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{
		ID:     token.ID,
		UsedAt: &usedAt,
	}); err != nil {
		t.Fatalf("second ConsumeMagicLinkToken: %v", err)
	}

	// Verify the token is marked used.
	fetched, err := s.GetMagicLinkTokenByHash(ctx, "hash-abc")
	if err != nil {
		t.Fatalf("GetMagicLinkTokenByHash: %v", err)
	}
	if fetched.UsedAt == nil {
		t.Error("expected UsedAt to be set, got nil")
	}
}

// TestDialectReportsSQLite is a sanity check that the SQLite adapter reports
// "sqlite" from Dialect().
func TestDialectReportsSQLite(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()
	if got := s.Dialect(); got != "sqlite" {
		t.Errorf("Dialect() = %q, want %q", got, "sqlite")
	}
}

// skipIfNoPGDSN skips the test when JAMSESH_TEST_PG_DSN is not set.
func skipIfNoPGDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("JAMSESH_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("JAMSESH_TEST_PG_DSN not set; skipping Postgres tests")
	}
	return dsn
}

// TestOpenPostgres verifies db.Open("postgres", dsn) returns a working Store.
func TestOpenPostgres(t *testing.T) {
	dsn := skipIfNoPGDSN(t)
	ctx := context.Background()

	s, _, err := db.Open(ctx, "postgres", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open postgres: %v", err)
	}
	defer s.Close()

	if s.Dialect() != "postgres" {
		t.Fatalf("Dialect() = %q, want %q", s.Dialect(), "postgres")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
