package sessionresume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// exchangeGenericFailure is the single error shape returned for ALL token
// failures (expired / already-used / unknown). No oracle: callers cannot
// distinguish the three cases from the response — only from the audit log.
var exchangeGenericFailure = openapi.ExchangeSessionResume401JSONResponse{
	UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
		Error:   "auth.invalid_token",
		Message: "invalid or expired resume token",
	},
}

// ExchangeSessionResume implements POST /api/session-resumes/exchange.
//
// UNAUTHENTICATED: the resume token IS the credential. Any ambient
// Authorization header is intentionally ignored — the method reads
// nothing from the request context about an authenticated account.
//
// Security contract:
//   - The token is consumed atomically (winner-returning ConsumeResumeToken).
//     Exactly one concurrent exchange wins; the second receives the generic
//     failure.
//   - ALL failure modes (unknown, expired, already-used) return the same
//     generic 401 — no oracle that allows enumeration.
//   - The raw token is NEVER logged; only the hash appears in the DB.
func (h *Handler) ExchangeSessionResume(ctx context.Context, req openapi.ExchangeSessionResumeRequestObject) (openapi.ExchangeSessionResumeResponseObject, error) {
	rawToken := req.Body.ResumeToken

	// Hash the raw token for the atomic consume. The raw value is never
	// persisted or logged beyond this line.
	hash := hashResumeToken(rawToken)

	now := h.clock.Now()
	usedAt := now
	consumed, err := h.store.ConsumeResumeToken(ctx, store.ConsumeResumeTokenParams{
		TokenHash: hash,
		UsedAt:    &usedAt,
		Now:       now,
	})
	if err != nil {
		if isNotFound(err) {
			// Token is missing, expired, or already used. Return the generic
			// failure — no distinguishing detail.
			return exchangeGenericFailure, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: consume resume token: %w", err))
	}

	// Fetch the account bound to the consumed token so we can branch on
	// is_anonymous and get the display_name.
	acct, err := h.store.GetAccountByID(ctx, consumed.AccountID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: get account: %w", err))
	}

	if acct.IsAnonymous {
		return h.issuePlaygroundCredential(ctx, consumed, acct, now)
	}
	return h.issueDurableCredential(ctx, consumed, acct)
}

// issuePlaygroundCredential mints an anonymous-session bearer for the
// existing anonymous account. The bearer TTL is the remaining time until the
// session's hard-cap (HardCapAt). Falls back to resumeTokenTTL when the
// session row cannot be fetched or has no HardCapAt.
func (h *Handler) issuePlaygroundCredential(ctx context.Context, consumed store.ResumeToken, acct store.Account, now time.Time) (openapi.ExchangeSessionResumeResponseObject, error) {
	// Fetch the session to determine the remaining hard-cap TTL.
	ttl, err := h.remainingSessionTTL(ctx, consumed.OrgID, consumed.SessionID, now)
	if err != nil {
		return nil, fmt.Errorf("sessionresume: playground credential: %w", err)
	}
	if ttl <= 0 {
		// Session is already ended / past hard-cap — generic failure.
		return exchangeGenericFailure, nil
	}

	rawBearer, expiresAt, err := h.tokens.IssueAnonymousSessionBearerForExistingAccount(
		ctx, acct.ID, consumed.SessionID, ttl,
	)
	if err != nil {
		if isTokenForbidden(err) {
			// Account turned non-anonymous between mint and exchange — paranoia
			// guard; treat as generic failure.
			return exchangeGenericFailure, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: issue playground bearer: %w", err))
	}

	return openapi.ExchangeSessionResume200JSONResponse(openapi.SessionResumeExchangeResponse{
		Bearer:      rawBearer,
		ExpiresAt:   expiresAt,
		SessionId:   consumed.SessionID,
		OrgId:       consumed.OrgID,
		Kind:        openapi.Playground,
		AccountId:   acct.ID,
		DisplayName: acct.DisplayName,
	}), nil
}

// issueDurableCredential mints a short-lived access token (no refresh) for
// the durable account.
func (h *Handler) issueDurableCredential(ctx context.Context, consumed store.ResumeToken, acct store.Account) (openapi.ExchangeSessionResumeResponseObject, error) {
	rawBearer, expiresAt, err := h.tokens.IssueShortLived(ctx, acct.ID, tokens.AccessTokenTTL)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: issue durable token: %w", err))
	}

	return openapi.ExchangeSessionResume200JSONResponse(openapi.SessionResumeExchangeResponse{
		Bearer:      rawBearer,
		ExpiresAt:   expiresAt,
		SessionId:   consumed.SessionID,
		OrgId:       consumed.OrgID,
		Kind:        openapi.Durable,
		AccountId:   acct.ID,
		DisplayName: acct.DisplayName,
	}), nil
}

// remainingSessionTTL returns how much time remains until the session's
// HardCapAt, or a sane fallback when the session has no hard cap. Returns a
// non-positive value when the session is ended or the hard cap has passed.
func (h *Handler) remainingSessionTTL(ctx context.Context, orgID, sessionID string, now time.Time) (time.Duration, error) {
	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if isNotFound(err) {
			// Session row was deleted (e.g. destroyed after mint).
			return 0, nil
		}
		return 0, fmt.Errorf("get session: %w", err)
	}
	if sess.Status == "ended" || sess.Status == "archived" {
		return 0, nil
	}
	if sess.HardCapAt != nil {
		remaining := sess.HardCapAt.Sub(now)
		if remaining <= 0 {
			return 0, nil
		}
		return remaining, nil
	}
	// No hard cap (durable-ish playground session, or session without deadline):
	// fall back to resumeTokenTTL so the credential isn't longer-lived than the
	// mint operation that triggered this exchange.
	return resumeTokenTTL, nil
}

// hashResumeToken returns the SHA-256 hex digest of the raw token string.
// Mirrors generateResumeToken's hash step so the exchange path can look up
// the consumed row without persisting the raw value.
func hashResumeToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// isNotFound returns true when err wraps store.ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}

// isTokenForbidden returns true when err is tokens.ErrForbidden (account is
// not anonymous — durable account supplied to the playground credential path).
func isTokenForbidden(err error) bool {
	return errors.Is(err, tokens.ErrForbidden)
}
