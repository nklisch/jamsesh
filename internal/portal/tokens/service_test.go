package tokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// fakeClock allows tests to control the current time.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

// openStore opens a fresh in-memory SQLite store with migrations applied.
func openStore(t *testing.T) store.Store {
	t.Helper()
	s, err := db.Open(context.Background(), "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite :memory:: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// mustCreateAccount inserts a minimal account row and returns it.
func mustCreateAccount(t *testing.T, ctx context.Context, s store.Store, email string) store.Account {
	t.Helper()
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          email + "-id",
		Email:       email,
		DisplayName: email,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustCreateAccount(%q): %v", email, err)
	}
	return acc
}

func TestService_Issue_ReturnsPair(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "alice@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(pair.AccessToken) != 64 {
		t.Errorf("AccessToken length: want 64, got %d", len(pair.AccessToken))
	}
	if len(pair.RefreshToken) != 64 {
		t.Errorf("RefreshToken length: want 64, got %d", len(pair.RefreshToken))
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Error("AccessToken and RefreshToken must differ")
	}
	if !pair.AccessExpiresAt.After(time.Now()) {
		t.Error("AccessExpiresAt should be in the future")
	}
	if !pair.RefreshExpiresAt.After(pair.AccessExpiresAt) {
		t.Error("RefreshExpiresAt should be after AccessExpiresAt")
	}
}

func TestService_Issue_WritesTwoRows(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "bob@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	rows, err := s.ListOAuthTokensForAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("ListOAuthTokensForAccount: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 token rows, got %d", len(rows))
	}

	kinds := map[string]bool{}
	for _, r := range rows {
		kinds[r.Kind] = true
	}
	if !kinds["access"] {
		t.Error("missing 'access' kind row")
	}
	if !kinds["refresh"] {
		t.Error("missing 'refresh' kind row")
	}

	// Verify the tokens are not stored in plain text
	for _, r := range rows {
		if r.TokenHash == pair.AccessToken || r.TokenHash == pair.RefreshToken {
			t.Error("raw token stored in token_hash — must be hashed")
		}
	}
}

func TestService_Validate_Valid(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "carol@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := svc.Validate(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("Validate valid token: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("Validate returned wrong account: got %q, want %q", got.ID, acc.ID)
	}
}

func TestService_Validate_UnknownToken(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, err := svc.Validate(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !errors.Is(err, tokens.ErrInvalidToken) {
		t.Errorf("unknown token: want ErrInvalidToken, got %v", err)
	}
}

func TestService_Validate_EmptyToken(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, err := svc.Validate(ctx, "")
	if !errors.Is(err, tokens.ErrInvalidToken) {
		t.Errorf("empty token: want ErrInvalidToken, got %v", err)
	}
}

func TestService_Validate_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "dave@example.com")

	clk := &fakeClock{t: time.Now().UTC()}
	svc := tokens.NewWithClock(s, clk)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Advance past AccessTokenTTL.
	clk.advance(tokens.AccessTokenTTL + time.Second)

	_, err = svc.Validate(ctx, pair.AccessToken)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("expired token: want ErrExpiredToken, got %v", err)
	}
}

func TestService_Validate_RevokedToken(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "eve@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := svc.Revoke(ctx, pair.AccessToken, false); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err = svc.Validate(ctx, pair.AccessToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("revoked token: want ErrRevokedToken, got %v", err)
	}
}

func TestService_Refresh_MintsNewPair(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "frank@example.com")
	svc := tokens.New(s)

	pair1, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	pair2, err := svc.Refresh(ctx, pair1.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// New pair must have different tokens.
	if pair2.AccessToken == pair1.AccessToken {
		t.Error("Refresh returned same access token")
	}
	if pair2.RefreshToken == pair1.RefreshToken {
		t.Error("Refresh returned same refresh token")
	}

	// New access token must be valid.
	got, err := svc.Validate(ctx, pair2.AccessToken)
	if err != nil {
		t.Fatalf("Validate new access token: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("new access token: wrong account: got %q, want %q", got.ID, acc.ID)
	}
}

func TestService_Refresh_OldRefreshRevoked(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "grace@example.com")
	svc := tokens.New(s)

	pair1, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := svc.Refresh(ctx, pair1.RefreshToken); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// The old refresh token must now be revoked.
	_, err = svc.Validate(ctx, pair1.RefreshToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("old refresh token after Refresh: want ErrRevokedToken, got %v", err)
	}
}

func TestService_Refresh_WithAccessTokenFails(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "henry@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Refreshing with an access token must fail.
	_, err = svc.Refresh(ctx, pair.AccessToken)
	if !errors.Is(err, tokens.ErrInvalidToken) {
		t.Errorf("Refresh with access token: want ErrInvalidToken, got %v", err)
	}
}

func TestService_Revoke_Single(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "irene@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := svc.Revoke(ctx, pair.AccessToken, false); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Access token revoked.
	_, err = svc.Validate(ctx, pair.AccessToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("after single revoke: access token: want ErrRevokedToken, got %v", err)
	}

	// Refresh token should still be valid.
	got, err := svc.Validate(ctx, pair.RefreshToken)
	if err != nil {
		t.Errorf("after single revoke: refresh token should still be valid, got %v", err)
	}
	if got != nil && got.ID != acc.ID {
		t.Errorf("wrong account on refresh token: got %q, want %q", got.ID, acc.ID)
	}
}

func TestService_Revoke_All(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "jack@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := svc.Revoke(ctx, pair.AccessToken, true); err != nil {
		t.Fatalf("Revoke(all): %v", err)
	}

	// Both tokens revoked.
	_, err = svc.Validate(ctx, pair.AccessToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("after revokeAll: access token: want ErrRevokedToken, got %v", err)
	}
	_, err = svc.Validate(ctx, pair.RefreshToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("after revokeAll: refresh token: want ErrRevokedToken, got %v", err)
	}
}

func TestService_Revoke_Idempotent(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	// Revoking an unknown token must not error.
	err := svc.Revoke(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	if err != nil {
		t.Errorf("Revoke unknown token: want nil, got %v", err)
	}
}

func TestService_Validate_RefreshTokenExpired(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "kate@example.com")

	clk := &fakeClock{t: time.Now().UTC()}
	svc := tokens.NewWithClock(s, clk)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Advance past RefreshTokenTTL.
	clk.advance(tokens.RefreshTokenTTL + time.Second)

	_, err = svc.Validate(ctx, pair.RefreshToken)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("expired refresh token: want ErrExpiredToken, got %v", err)
	}
}

func TestService_IssueShortLived_ReturnsValidToken(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "shortlived@example.com")
	svc := tokens.New(s)

	raw, expiresAt, err := svc.IssueShortLived(ctx, acc.ID, 5*time.Minute)
	if err != nil {
		t.Fatalf("IssueShortLived: %v", err)
	}
	if len(raw) != 64 {
		t.Errorf("short-lived token length: want 64, got %d", len(raw))
	}
	if !expiresAt.After(time.Now().Add(4 * time.Minute)) {
		t.Errorf("ExpiresAt %v should be ~5 min in the future", expiresAt)
	}

	// Validate accepts the token immediately.
	got, err := svc.Validate(ctx, raw)
	if err != nil {
		t.Fatalf("Validate just after issuance: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("Validate returned account %q, want %q", got.ID, acc.ID)
	}
}

func TestService_IssueShortLived_RejectedAfterTTL(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	acc := mustCreateAccount(t, ctx, s, "expired-short@example.com")

	clk := &fakeClock{t: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)}
	svc := tokens.NewWithClock(s, clk)

	raw, _, err := svc.IssueShortLived(ctx, acc.ID, 5*time.Minute)
	if err != nil {
		t.Fatalf("IssueShortLived: %v", err)
	}

	// Advance past the 5-minute TTL.
	clk.advance(5*time.Minute + time.Second)

	_, err = svc.Validate(ctx, raw)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("Validate after TTL: want ErrExpiredToken, got %v", err)
	}
}
