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
	// IssueShortLived mints a single bound access token with a caller-supplied
	// TTL. Used for ephemeral fetch-only credentials (e.g. finalize-run git
	// fetch). No refresh token is issued. Validation uses the same per-row
	// expiry path as Issue — Validate honours the row's expires_at without
	// special-casing.
	IssueShortLived(ctx context.Context, accountID string, ttl time.Duration) (accessRaw string, accessExpiresAt time.Time, err error)
	// IssueAnonymousSessionBearer creates a fresh anonymous account row (with a
	// synthetic email) and a session-scoped bearer for it in a single transaction.
	// The bearer's expires_at is set to now+ttl; the caller computes ttl from
	// the session's hard-cap deadline. Returns the unhashed rawToken (to return
	// to the client), the generated accountID (anon_* prefix), and expiresAt.
	// An empty nickname or sessionID is rejected before any DB calls are made.
	IssueAnonymousSessionBearer(ctx context.Context, sessionID, nickname string, ttl time.Duration) (rawToken, accountID string, expiresAt time.Time, err error)
	// Validate returns the account associated with a raw token, or a
	// normalized error (ErrInvalidToken, ErrExpiredToken, ErrRevokedToken).
	Validate(ctx context.Context, rawToken string) (*store.Account, error)
	// Refresh consumes the given refresh token (revoking it) and mints a new
	// pair with sliding-window TTLs.
	Refresh(ctx context.Context, refreshToken string) (Pair, error)
	// Revoke marks the supplied token as revoked. When revokeAll is true,
	// every token for the token's account is revoked (logout-everywhere).
	// callerAccountID is the bearer-authenticated account performing the
	// revocation; if it does not match the token's owner, ErrForbidden is
	// returned so callers can emit a 403.
	Revoke(ctx context.Context, callerAccountID string, rawToken string, revokeAll bool) error
}

// Sentinel errors that callers map to PROTOCOL.md error codes.
var (
	ErrInvalidToken = errors.New("tokens: invalid")
	ErrExpiredToken = errors.New("tokens: expired")
	ErrRevokedToken = errors.New("tokens: revoked")
	ErrForbidden    = errors.New("tokens: forbidden")
)
