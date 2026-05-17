package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/tokens"
)

const (
	magicLinkTTL        = 15 * time.Minute
	magicLinkTokenBytes = 32
	magicLinkSubject    = "Sign in to jamsesh"
)

// MagicLinkHandler handles the magic-link request and exchange endpoints.
// It satisfies the oapi-codegen StrictServerInterface methods for those two
// operations; main.go mixes it into the shared strict handler.
type MagicLinkHandler struct {
	store     store.Store
	tokensSvc tokens.Service
	sender    senders.Sender
	portalURL string // e.g. "https://example.com"
}

// NewMagicLinkHandler constructs a MagicLinkHandler.
func NewMagicLinkHandler(
	s store.Store,
	tokensSvc tokens.Service,
	sender senders.Sender,
	portalURL string,
) *MagicLinkHandler {
	return &MagicLinkHandler{
		store:     s,
		tokensSvc: tokensSvc,
		sender:    sender,
		portalURL: portalURL,
	}
}

// RequestMagicLink implements POST /api/auth/magic-link/request.
// Returns 204 on success. Sending errors are surfaced to the caller via a
// wrapped error so the strict handler returns 500 — better than silently
// swallowing delivery failures.
func (h *MagicLinkHandler) RequestMagicLink(
	ctx context.Context,
	req openapi.RequestMagicLinkRequestObject,
) (openapi.RequestMagicLinkResponseObject, error) {
	raw, hash, err := generateMagicToken()
	if err != nil {
		return nil, fmt.Errorf("magic-link: generate token: %w", err)
	}

	now := time.Now().UTC()
	email := string(req.Body.Email)
	if _, err := h.store.CreateMagicLinkToken(ctx, store.CreateMagicLinkTokenParams{
		ID:        uuid.New().String(),
		TokenHash: hash,
		Email:     email,
		IssuedAt:  now,
		ExpiresAt: now.Add(magicLinkTTL),
		UsedAt:    nil,
	}); err != nil {
		return nil, fmt.Errorf("magic-link: store token: %w", err)
	}

	magicURL := h.portalURL + "/auth/magic-link?token=" + raw
	body := "Click the link below to sign in to jamsesh:\n\n" + magicURL +
		"\n\nThis link expires in 15 minutes and can only be used once.\n"

	if err := h.sender.Send(ctx, email, magicLinkSubject, body); err != nil {
		return nil, fmt.Errorf("magic-link: send email: %w", err)
	}

	return openapi.RequestMagicLink204Response{}, nil
}

// ExchangeMagicLink implements POST /api/auth/magic-link/exchange.
// Returns 200 with a TokenPair on success, 401 on token not found / expired /
// already used.
func (h *MagicLinkHandler) ExchangeMagicLink(
	ctx context.Context,
	req openapi.ExchangeMagicLinkRequestObject,
) (openapi.ExchangeMagicLinkResponseObject, error) {
	hash := hashMagicToken(req.Body.Token)

	row, err := h.store.GetMagicLinkTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return magicLinkUnauthorized("auth.invalid_token", "invalid or expired token"), nil
		}
		return nil, fmt.Errorf("magic-link: lookup token: %w", err)
	}

	now := time.Now().UTC()

	if now.After(row.ExpiresAt) {
		return magicLinkUnauthorized("auth.expired_token", "magic link has expired"), nil
	}
	if row.UsedAt != nil {
		return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil
	}

	// Consume the token (SQL-level UPDATE WHERE used_at IS NULL — race-safe).
	if err := h.store.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{
		ID:     row.ID,
		UsedAt: &now,
	}); err != nil {
		// If the consume fails it may be a concurrent exchange that won the race.
		return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil
	}

	id := Identity{
		Provider:    "magic-link",
		Email:       row.Email,
		DisplayName: emailPrefix(row.Email),
	}
	acc, _, err := FindOrProvision(ctx, h.store, id)
	if err != nil {
		return nil, fmt.Errorf("magic-link: provision account: %w", err)
	}

	pair, err := h.tokensSvc.Issue(ctx, acc.ID)
	if err != nil {
		return nil, fmt.Errorf("magic-link: issue tokens: %w", err)
	}

	return openapi.ExchangeMagicLink200JSONResponse{
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt,
		RefreshExpiresAt: pair.RefreshExpiresAt,
	}, nil
}

// --- helpers ----------------------------------------------------------------

func generateMagicToken() (raw, hash string, err error) {
	b := make([]byte, magicLinkTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand read: %w", err)
	}
	raw = hex.EncodeToString(b)
	hash = hashMagicToken(raw)
	return raw, hash, nil
}

func hashMagicToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func magicLinkUnauthorized(code, message string) openapi.ExchangeMagicLinkResponseObject {
	return openapi.ExchangeMagicLink401JSONResponse{
		UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
			Error:   code,
			Message: message,
		},
	}
}
