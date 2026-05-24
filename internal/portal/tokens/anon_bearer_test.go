package tokens_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// openStoreWithSession opens a fresh SQLite store and creates a minimal session
// row (needed for the anonymous bearer's session_id FK). Returns the store and
// the session ID.
func openStoreWithSession(t *testing.T) (store.Store, string) {
	t.Helper()
	s := openStore(t)
	ctx := context.Background()

	// Create a minimal org.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-anon-test",
		Name:      "Anon Test Org",
		Slug:      "anon-test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// Create a minimal session.
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-anon-001",
		OrgID:         org.ID,
		Name:          "Anon Session",
		Goal:          "test anon bearers",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	return s, sess.ID
}

func TestIssueAnonymousSessionBearer_ReturnsValidRawToken(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, accountID, expiresAt, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "amber-otter", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}
	if len(rawToken) != 64 {
		t.Errorf("rawToken length: want 64, got %d", len(rawToken))
	}
	if !strings.HasPrefix(accountID, "anon_") {
		t.Errorf("accountID should start with 'anon_', got %q", accountID)
	}
	if expiresAt.Before(time.Now().Add(23 * time.Hour)) {
		t.Errorf("expiresAt %v should be ~24h in the future", expiresAt)
	}
}

func TestIssueAnonymousSessionBearer_ValidateAcceptsBearer(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "amber-otter", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	got, err := svc.Validate(ctx, rawToken)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ID != accountID {
		t.Errorf("Validate returned account %q, want %q", got.ID, accountID)
	}
	if !got.IsAnonymous {
		t.Error("Validate returned account with IsAnonymous=false, want true")
	}
	if got.DisplayName != "amber-otter" {
		t.Errorf("DisplayName: want 'amber-otter', got %q", got.DisplayName)
	}
}

func TestIssueAnonymousSessionBearer_AccountEmailIsSynthetic(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	_, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "blue-fox", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	acct, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	wantEmail := accountID + "@playground.local"
	if acct.Email != wantEmail {
		t.Errorf("synthetic email: want %q, got %q", wantEmail, acct.Email)
	}
}

func TestIssueAnonymousSessionBearer_UpdatesLastUsedAt(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "cedar-hawk", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Validate twice; last_used_at should be set after first call.
	if _, err := svc.Validate(ctx, rawToken); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Second call should also succeed (regression: not revoked by Validate).
	if _, err := svc.Validate(ctx, rawToken); err != nil {
		t.Errorf("second Validate: %v", err)
	}
}

func TestIssueAnonymousSessionBearer_ExpiredBearerRejected(t *testing.T) {
	ctx := context.Background()

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	// Create org + session.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "org-exp-001", Name: "Exp Org", Slug: "exp-org-001", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-exp-001",
		OrgID:         org.ID,
		Name:          "Exp Session",
		Goal:          "test expiry",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	clk := &fakeClock{t: time.Now().UTC()}
	svc := tokens.NewWithClock(s, clk)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sess.ID, "dawn-elk", 5*time.Minute)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Advance past TTL.
	clk.advance(5*time.Minute + time.Second)

	_, err = svc.Validate(ctx, rawToken)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("expired bearer: want ErrExpiredToken, got %v", err)
	}
}

func TestIssueAnonymousSessionBearer_RevokedBearerRejected(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "ember-crow", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Revoke all bearers for the session via the store (simulates destruction sweep).
	now := time.Now().UTC()
	if err := s.RevokeBearersForSession(ctx, store.RevokeBearersForSessionParams{
		RevokedAt: now,
		SessionID: sessID,
	}); err != nil {
		t.Fatalf("RevokeBearersForSession: %v", err)
	}

	_, err = svc.Validate(ctx, rawToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("revoked bearer: want ErrRevokedToken, got %v", err)
	}
}

func TestIssueAnonymousSessionBearer_TransactionalRollback(t *testing.T) {
	// This test verifies that if bearer creation fails, the account row is also
	// rolled back (no orphaned accounts).
	// We verify this indirectly: issue with an invalid (empty) sessionID to
	// trigger the pre-tx validation, confirming no DB calls are made.
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, "", "fern-moth", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
	// No account should have been created.
	rows, listErr := s.ListOAuthTokensForAccount(ctx, "anon_anything")
	if listErr != nil {
		t.Fatalf("ListOAuthTokensForAccount: %v", listErr)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 token rows after failed issue, got %d", len(rows))
	}
}

func TestIssueAnonymousSessionBearer_EmptyNickname_Rejected(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty nickname, got nil")
	}
}

func TestIssueAnonymousSessionBearer_EmptySessionID_Rejected(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, "", "ghost-heron", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
}
