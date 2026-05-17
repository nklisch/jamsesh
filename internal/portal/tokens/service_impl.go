package tokens

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/db/store"
)

// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate expiry.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type service struct {
	store store.Store
	clock Clock
}

// New returns a Service backed by the given Store using the real system clock.
func New(s store.Store) Service {
	return &service{store: s, clock: realClock{}}
}

// NewWithClock returns a Service with an injected clock. Intended for tests.
func NewWithClock(s store.Store, c Clock) Service {
	return &service{store: s, clock: c}
}

func (s *service) Issue(ctx context.Context, accountID string) (Pair, error) {
	now := s.clock.Now()

	accessRaw, accessHash, err := generateToken()
	if err != nil {
		return Pair{}, err
	}
	refreshRaw, refreshHash, err := generateToken()
	if err != nil {
		return Pair{}, err
	}

	accessExpiry := now.Add(AccessTokenTTL)
	refreshExpiry := now.Add(RefreshTokenTTL)

	if _, err := s.store.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
		ID:        uuid.New().String(),
		AccountID: accountID,
		TokenHash: accessHash,
		Kind:      "access",
		IssuedAt:  now,
		ExpiresAt: accessExpiry,
	}); err != nil {
		return Pair{}, err
	}

	if _, err := s.store.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
		ID:        uuid.New().String(),
		AccountID: accountID,
		TokenHash: refreshHash,
		Kind:      "refresh",
		IssuedAt:  now,
		ExpiresAt: refreshExpiry,
	}); err != nil {
		return Pair{}, err
	}

	return Pair{
		AccessToken:      accessRaw,
		RefreshToken:     refreshRaw,
		AccessExpiresAt:  accessExpiry,
		RefreshExpiresAt: refreshExpiry,
	}, nil
}

func (s *service) Validate(ctx context.Context, raw string) (*store.Account, error) {
	if raw == "" {
		return nil, ErrInvalidToken
	}

	row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	now := s.clock.Now()

	// Check revocation before expiry — revoked takes priority.
	if row.RevokedAt != nil {
		return nil, ErrRevokedToken
	}
	if now.After(row.ExpiresAt) {
		return nil, ErrExpiredToken
	}

	// Touch last_used_at fire-and-forget — validation correctness does not
	// depend on this succeeding.
	now2 := now
	_ = s.store.TouchOAuthTokenLastUsed(ctx, store.TouchOAuthTokenLastUsedParams{
		ID:         row.ID,
		LastUsedAt: &now2,
	})

	acct, err := s.store.GetAccountByID(ctx, row.AccountID)
	if err != nil {
		return nil, err
	}
	return &acct, nil
}

func (s *service) Refresh(ctx context.Context, raw string) (Pair, error) {
	// Validate first to check expiry and revocation.
	acct, err := s.Validate(ctx, raw)
	if err != nil {
		return Pair{}, err
	}

	// Re-fetch the row to confirm it's a refresh token.
	row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
	if err != nil {
		return Pair{}, err
	}
	if row.Kind != "refresh" {
		return Pair{}, ErrInvalidToken
	}

	// Revoke the consumed refresh token (single-use sliding-window).
	now := s.clock.Now()
	if err := s.store.RevokeOAuthToken(ctx, store.RevokeOAuthTokenParams{
		ID:        row.ID,
		RevokedAt: &now,
	}); err != nil {
		return Pair{}, err
	}

	return s.Issue(ctx, acct.ID)
}

func (s *service) Revoke(ctx context.Context, raw string, revokeAll bool) error {
	row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // idempotent: already gone
		}
		return err
	}

	now := s.clock.Now()
	if revokeAll {
		return s.store.RevokeAllOAuthTokensForAccount(ctx, store.RevokeAllOAuthTokensForAccountParams{
			AccountID: row.AccountID,
			RevokedAt: &now,
		})
	}
	return s.store.RevokeOAuthToken(ctx, store.RevokeOAuthTokenParams{
		ID:        row.ID,
		RevokedAt: &now,
	})
}
