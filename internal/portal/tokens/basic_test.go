package tokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/portal/tokens"
)

func TestBasicAuthValidator_ValidToken(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	acc := mustCreateAccount(t, ctx, s, "validator@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	validate := tokens.BasicAuthValidator(svc)
	got, err := validate(ctx, "git", pair.AccessToken)
	if err != nil {
		t.Fatalf("BasicAuthValidator valid token: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("wrong account: got %q, want %q", got.ID, acc.ID)
	}
}

func TestBasicAuthValidator_InvalidToken(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	svc := tokens.New(s)
	validate := tokens.BasicAuthValidator(svc)

	_, err = validate(ctx, "git", "notavalidtoken0000000000000000000000000000000000000000000000000000")
	if !errors.Is(err, tokens.ErrInvalidToken) {
		t.Errorf("invalid token: want ErrInvalidToken, got %v", err)
	}
}

func TestBasicAuthValidator_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	acc := mustCreateAccount(t, ctx, s, "expiry@example.com")
	clk := &fakeClock{t: mustNow()}
	svc := tokens.NewWithClock(s, clk)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	clk.advance(tokens.AccessTokenTTL + 1)

	validate := tokens.BasicAuthValidator(svc)
	_, err = validate(ctx, "git", pair.AccessToken)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("expired token: want ErrExpiredToken, got %v", err)
	}
}

func TestBasicAuthValidator_RevokedToken(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	acc := mustCreateAccount(t, ctx, s, "revoked@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if err := svc.Revoke(ctx, acc.ID, pair.AccessToken, false); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	validate := tokens.BasicAuthValidator(svc)
	_, err = validate(ctx, "git", pair.AccessToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("revoked token: want ErrRevokedToken, got %v", err)
	}
}

func TestBasicAuthValidator_UsernameIgnored(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	acc := mustCreateAccount(t, ctx, s, "ignored@example.com")
	svc := tokens.New(s)

	pair, err := svc.Issue(ctx, acc.ID)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	validate := tokens.BasicAuthValidator(svc)

	// Username should not matter
	for _, username := range []string{"", "git", "user", "anything"} {
		got, err := validate(ctx, username, pair.AccessToken)
		if err != nil {
			t.Errorf("username=%q: unexpected error: %v", username, err)
		}
		if got == nil || got.ID != acc.ID {
			t.Errorf("username=%q: wrong account", username)
		}
	}
}

func mustNow() time.Time { return time.Now().UTC() }
