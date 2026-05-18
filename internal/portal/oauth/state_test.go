package oauth_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/oauth"
)

// openOAuthStore opens an in-memory SQLite store for state-table tests.
func openOAuthStore(t *testing.T) store.Store {
	t.Helper()
	s, _, err := db.Open(context.Background(), "sqlite", "file::memory:?cache=shared", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestStoreStateAt_UsesSuppliedClock asserts that the inserted row's
// CreatedAt equals the passed-in now, and ExpiresAt equals now +
// StateNonceTTL — proving the clock parameter is the sole source of
// time for both stamps.
func TestStoreStateAt_UsesSuppliedClock(t *testing.T) {
	s := openOAuthStore(t)
	ctx := context.Background()

	frozen := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	nonce := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	if err := oauth.StoreStateAt(ctx, s, nonce, "github",
		"https://portal.example.com/oauth/callback", frozen); err != nil {
		t.Fatalf("StoreStateAt: %v", err)
	}

	// Consume to read back the row.
	row, err := s.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}

	if !row.CreatedAt.Equal(frozen) {
		t.Errorf("CreatedAt: want %v, got %v", frozen, row.CreatedAt)
	}
	wantExpires := frozen.Add(oauth.StateNonceTTL)
	if !row.ExpiresAt.Equal(wantExpires) {
		t.Errorf("ExpiresAt: want %v, got %v", wantExpires, row.ExpiresAt)
	}
	if row.Provider != "github" {
		t.Errorf("Provider: want github, got %q", row.Provider)
	}
	if row.RedirectURI != "https://portal.example.com/oauth/callback" {
		t.Errorf("RedirectURI: got %q", row.RedirectURI)
	}
}

// TestStoreState_DelegatesToStoreStateAt asserts that the back-compat
// StoreState entry point produces a row whose CreatedAt is close to the
// real wall clock (sanity check; not an exact equality because real
// time advances during the call).
func TestStoreState_DelegatesToStoreStateAt(t *testing.T) {
	s := openOAuthStore(t)
	ctx := context.Background()

	nonce := "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"
	before := time.Now().UTC()
	if err := oauth.StoreState(ctx, s, nonce, "github",
		"https://portal.example.com/oauth/callback"); err != nil {
		t.Fatalf("StoreState: %v", err)
	}
	after := time.Now().UTC()

	row, err := s.ConsumeOAuthState(ctx, nonce)
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}

	if row.CreatedAt.Before(before) || row.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in [%v, %v]", row.CreatedAt, before, after)
	}
	wantExpiresLow := before.Add(oauth.StateNonceTTL)
	wantExpiresHigh := after.Add(oauth.StateNonceTTL)
	if row.ExpiresAt.Before(wantExpiresLow) || row.ExpiresAt.After(wantExpiresHigh) {
		t.Errorf("ExpiresAt %v not in [%v, %v]", row.ExpiresAt, wantExpiresLow, wantExpiresHigh)
	}
}
