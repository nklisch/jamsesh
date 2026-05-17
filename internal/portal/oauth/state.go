package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"jamsesh/internal/db/store"
)

const (
	// StateNonceTTL is how long a state nonce is valid after creation.
	StateNonceTTL = 5 * time.Minute
	// stateNonceBytes is the entropy size of the nonce before hex-encoding.
	stateNonceBytes = 32
)

// GenerateNonce generates a cryptographically random 32-byte nonce,
// hex-encoded as a 64-character string.
func GenerateNonce() (string, error) {
	b := make([]byte, stateNonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: generate nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// StoreState inserts a fresh state nonce into oauth_state with a
// StateNonceTTL expiry. The nonce is consumed atomically on callback via
// ConsumeState. Uses the real system clock; clock-injectable callers
// should use StoreStateAt.
func StoreState(ctx context.Context, s store.OAuthStateStore, nonce, provider, redirectURI string) error {
	return StoreStateAt(ctx, s, nonce, provider, redirectURI, time.Now().UTC())
}

// StoreStateAt inserts a fresh state nonce using the supplied timestamp
// as CreatedAt. ExpiresAt is now + StateNonceTTL. Used by
// clock-injectable callers so test-clock advancement is observable in
// both stamps.
func StoreStateAt(ctx context.Context, s store.OAuthStateStore, nonce, provider, redirectURI string, now time.Time) error {
	return s.InsertOAuthState(ctx, store.InsertOAuthStateParams{
		Nonce:       nonce,
		Provider:    provider,
		RedirectURI: redirectURI,
		CreatedAt:   now,
		ExpiresAt:   now.Add(StateNonceTTL),
	})
}

// ConsumeState atomically deletes and returns the state row identified by
// nonce. Returns store.ErrNotFound when the nonce does not exist (already
// consumed, never issued, or DB error).
func ConsumeState(ctx context.Context, s store.OAuthStateStore, nonce string) (store.OAuthState, error) {
	return s.ConsumeOAuthState(ctx, nonce)
}
