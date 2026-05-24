package tokens

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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

// tokensStore is the minimal store interface consumed by service.
type tokensStore interface {
	store.OAuthTokenStore
	store.AccountStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

type service struct {
	store tokensStore
	clock Clock
}

// New returns a Service backed by the given Store using the real system clock.
func New(s tokensStore) Service {
	return &service{store: s, clock: realClock{}}
}

// NewWithClock returns a Service with an injected clock. Intended for tests.
func NewWithClock(s tokensStore, c Clock) Service {
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

// IssueShortLived mints a single access token bound to accountID with the
// supplied TTL. No refresh token is issued — short-lived tokens are intended
// for ephemeral flows (e.g. git fetch in finalize-run) where the credential
// cannot be refreshed.
func (s *service) IssueShortLived(ctx context.Context, accountID string, ttl time.Duration) (string, time.Time, error) {
	now := s.clock.Now()

	accessRaw, accessHash, err := generateToken()
	if err != nil {
		return "", time.Time{}, err
	}

	accessExpiry := now.Add(ttl)

	if _, err := s.store.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
		ID:        uuid.New().String(),
		AccountID: accountID,
		TokenHash: accessHash,
		Kind:      "access",
		IssuedAt:  now,
		ExpiresAt: accessExpiry,
	}); err != nil {
		return "", time.Time{}, err
	}

	return accessRaw, accessExpiry, nil
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

// randID generates a crypto-random URL-safe hex ID of the specified byte
// length. The returned string is 2*n characters long (hex-encoded).
func randID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("tokens: rand ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// IssueAnonymousSessionBearer creates a fresh anonymous account row and a
// session-scoped bearer for it in a single transaction. The bearer's expires_at
// is set to now+ttl. Returns the unhashed rawToken, the generated accountID,
// and expiresAt on success.
func (s *service) IssueAnonymousSessionBearer(ctx context.Context, sessionID, nickname string, ttl time.Duration) (string, string, time.Time, error) {
	if nickname == "" {
		return "", "", time.Time{}, errors.New("tokens: nickname must not be empty")
	}
	if sessionID == "" {
		return "", "", time.Time{}, errors.New("tokens: sessionID must not be empty")
	}

	idSuffix, err := randID(16)
	if err != nil {
		return "", "", time.Time{}, err
	}
	accountID := "anon_" + idSuffix
	email := accountID + "@playground.local"
	now := s.clock.Now().UTC()

	var rawToken string
	var expiresAt time.Time

	txErr := s.store.WithTx(ctx, func(q store.TxStore) error {
		_, err := q.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
			ID:          accountID,
			Email:       email,
			DisplayName: nickname,
			CreatedAt:   now,
		})
		if err != nil {
			return fmt.Errorf("create anon account: %w", err)
		}

		raw, hash, err := generateToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
		rawToken = raw
		expiresAt = now.Add(ttl)

		_, err = q.CreateAnonymousBearer(ctx, store.CreateAnonymousBearerParams{
			ID:        "tok_" + uuid.New().String(),
			AccountID: accountID,
			TokenHash: hash,
			SessionID: sessionID,
			IssuedAt:  now,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			return fmt.Errorf("create anon bearer: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return "", "", time.Time{}, txErr
	}

	return rawToken, accountID, expiresAt, nil
}

func (s *service) Revoke(ctx context.Context, callerAccountID string, raw string, revokeAll bool) error {
	row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // idempotent: already gone
		}
		return err
	}

	// Ownership check: prevent caller A from revoking caller B's tokens.
	if row.AccountID != callerAccountID {
		return ErrForbidden
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
