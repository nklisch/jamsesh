package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/tokens"
)

// reservedMagicLinkDomains lists email domain suffixes that are reserved for
// internal synthetic addresses (e.g. anonymous playground accounts). Magic-link
// requests using these domains are rejected with 400 magic_link.reserved_domain
// to prevent a user from colliding with a synthetically-issued identity.
var reservedMagicLinkDomains = []string{
	"@playground.local",
}

const (
	magicLinkTTL        = 15 * time.Minute
	magicLinkTokenBytes = 32
	magicLinkSubject    = "Sign in to jamsesh"
)

// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate expiry. The
// shape mirrors internal/portal/tokens.Clock by design — the same
// concrete type can satisfy both interfaces (handy for the e2etest-
// tagged AdvanceableClock).
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// magicLinkHandlerStore is the minimal store interface consumed by MagicLinkHandler.
type magicLinkHandlerStore interface {
	store.MagicLinkTokenStore
	provisionStore
}

// MagicLinkHandler handles the magic-link request and exchange endpoints.
// It satisfies the oapi-codegen StrictServerInterface methods for those two
// operations; main.go mixes it into the shared strict handler.
type MagicLinkHandler struct {
	store     magicLinkHandlerStore
	tokensSvc tokens.Service
	sender    senders.Sender
	portalURL string // e.g. "https://example.com"
	clock     Clock
}

// NewMagicLinkHandler constructs a MagicLinkHandler with the real system
// clock. Production callers use this.
func NewMagicLinkHandler(
	s magicLinkHandlerStore,
	tokensSvc tokens.Service,
	sender senders.Sender,
	portalURL string,
) *MagicLinkHandler {
	return NewMagicLinkHandlerWithClock(s, tokensSvc, sender, portalURL, realClock{})
}

// NewMagicLinkHandlerWithClock constructs a MagicLinkHandler with the supplied
// clock. Used by unit tests (fakeClock) and the e2etest-tagged binary
// (testclock.AdvanceableClock).
func NewMagicLinkHandlerWithClock(
	s magicLinkHandlerStore,
	tokensSvc tokens.Service,
	sender senders.Sender,
	portalURL string,
	clock Clock,
) *MagicLinkHandler {
	return &MagicLinkHandler{
		store:     s,
		tokensSvc: tokensSvc,
		sender:    sender,
		portalURL: portalURL,
		clock:     clock,
	}
}

// RequestMagicLink implements POST /api/auth/magic-link/request.
// Returns 204 on success. Sending errors are wrapped with
// deperr.WrapSMTP so the strict-handler translator emits a typed
// dep.smtp_unavailable 503 envelope instead of an opaque 500.
func (h *MagicLinkHandler) RequestMagicLink(
	ctx context.Context,
	req openapi.RequestMagicLinkRequestObject,
) (openapi.RequestMagicLinkResponseObject, error) {
	raw, hash, err := generateMagicToken()
	if err != nil {
		return nil, fmt.Errorf("magic-link: generate token: %w", err)
	}

	now := h.clock.Now()
	email := string(req.Body.Email)

	// Reject emails in reserved internal domains to prevent synthetic-account
	// collisions (e.g. anonymous playground accounts use @playground.local).
	emailLower := strings.ToLower(email)
	for _, domain := range reservedMagicLinkDomains {
		if strings.HasSuffix(emailLower, domain) {
			return nil, httperr.ErrReservedDomain()
		}
	}

	if _, err := h.store.CreateMagicLinkToken(ctx, store.CreateMagicLinkTokenParams{
		ID:        uuid.New().String(),
		TokenHash: hash,
		Email:     email,
		IssuedAt:  now,
		ExpiresAt: now.Add(magicLinkTTL),
		UsedAt:    nil,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: store token: %w", err))
	}

	magicURL := h.portalURL + "/auth/magic-link#token=" + raw
	body := "Click the link below to sign in to jamsesh:\n\n" + magicURL +
		"\n\nThis link expires in 15 minutes and can only be used once.\n"

	if err := h.sender.Send(ctx, email, magicLinkSubject, body); err != nil {
		if errors.Is(err, senders.ErrMagicLinkNotEnabled) {
			return nil, httperr.ErrMagicLinkNotEnabled()
		}
		return nil, deperr.WrapSMTP(fmt.Errorf("magic-link: send email: %w", err))
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: lookup token: %w", err))
	}

	now := h.clock.Now()

	if now.After(row.ExpiresAt) {
		return magicLinkUnauthorized("auth.expired_token", "magic link has expired"), nil
	}
	if row.UsedAt != nil {
		return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil
	}

	// Consume the token (SQL-level UPDATE WHERE used_at IS NULL — race-safe).
	// affected == 1: this caller won the race; proceed to provision + issue.
	// affected == 0: another exchange already consumed this token (race lost) → 401.
	// err != nil:    a real driver/transient failure → surface as 5xx.
	affected, err := h.store.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{
		ID:     row.ID,
		UsedAt: &now,
	})
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: consume token: %w", err))
	}
	if affected == 0 {
		return magicLinkUnauthorized("auth.invalid_token", "magic link already used"), nil
	}

	id := Identity{
		Provider:    "magic-link",
		Email:       row.Email,
		DisplayName: emailPrefix(row.Email),
	}
	acc, _, err := FindOrProvisionAt(ctx, h.store, id, now)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: provision account: %w", err))
	}

	pair, err := h.tokensSvc.Issue(ctx, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("magic-link: issue tokens: %w", err))
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
