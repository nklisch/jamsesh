// Package tokens implements the token subsystem for the portal: issuance,
// validation, refresh, and revocation of opaque OAuth tokens. Both Bearer
// (REST/MCP) and HTTP Basic (git smart-HTTP) transports consume a single
// Service so the auth codepath is unified.
package tokens

import (
	"context"
	"errors"
	"time"

	"jamsesh/internal/db/store"
)

// Lifetimes locked by SECURITY.md.
const (
	AccessTokenTTL  = 1 * time.Hour
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// Pair is the user-visible bundle returned on issuance and refresh.
type Pair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// Service is the unified token lifecycle interface.
type Service interface {
	// Issue mints a new access+refresh pair for the given account.
	Issue(ctx context.Context, accountID string) (Pair, error)
	// Validate returns the account associated with a raw token, or a
	// normalized error (ErrInvalidToken, ErrExpiredToken, ErrRevokedToken).
	Validate(ctx context.Context, rawToken string) (*store.Account, error)
	// Refresh consumes the given refresh token (revoking it) and mints a new
	// pair with sliding-window TTLs.
	Refresh(ctx context.Context, refreshToken string) (Pair, error)
	// Revoke marks the supplied token as revoked. When revokeAll is true,
	// every token for the token's account is revoked (logout-everywhere).
	Revoke(ctx context.Context, rawToken string, revokeAll bool) error
}

// Sentinel errors that callers map to PROTOCOL.md error codes.
var (
	ErrInvalidToken = errors.New("tokens: invalid")
	ErrExpiredToken = errors.New("tokens: expired")
	ErrRevokedToken = errors.New("tokens: revoked")
)
