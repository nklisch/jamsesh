package sessionresume

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// resumeTokenTTL is the lifetime of a single-use resume token.
// 60 seconds is long enough for the CLI to open the URL and the browser to
// load the SPA; short enough that a leaked URL is not useful to attackers.
const resumeTokenTTL = 60 * time.Second

// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle (sessionresume → playground would be
// cyclic). Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"

// rawTokenBytes is the number of random bytes drawn for each resume token.
// 32 bytes → 64 hex chars → 256 bits of entropy.
const rawTokenBytes = 32

// CreateSessionResume implements POST /api/session-resumes.
//
// Mints a single-use 60-second resume token bound to (account, org, session)
// and returns ONLY { resume_url, expires_in, session_id }. The raw token is
// embedded in the resume_url fragment (#rt=<token>) and is never returned as a
// standalone field, so it does not appear in server-side response logs.
func (h *Handler) CreateSessionResume(ctx context.Context, req openapi.CreateSessionResumeRequestObject) (openapi.CreateSessionResumeResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.CreateSessionResume401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.Body.OrgId
	sessionID := req.Body.SessionId

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.CreateSessionResume403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.CreateSessionResume403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.CreateSessionResume404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	// Generate a cryptographically random token.
	rawToken, tokenHash, err := generateResumeToken()
	if err != nil {
		return nil, fmt.Errorf("sessionresume: generate token: %w", err)
	}

	now := h.clock.Now()
	expiresAt := now.Add(resumeTokenTTL)
	id := ulid.Make().String()

	// Store the HASH only — the raw token must never be persisted.
	if _, err := h.store.CreateResumeToken(ctx, store.CreateResumeTokenParams{
		ID:        id,
		TokenHash: tokenHash,
		SessionID: sessionID,
		OrgID:     orgID,
		AccountID: acc.ID,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		UsedAt:    nil,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessionresume: store token: %w", err))
	}

	// Build the resume URL with the raw token in the fragment.
	// SECURITY: the raw token must only appear in the fragment — never in the
	// path, query, or any loggable part of the URL.
	resumeURL, err := composeResumeURL(h.portalURL, orgID, sessionID, rawToken)
	if err != nil {
		return nil, fmt.Errorf("sessionresume: compose resume URL: %w", err)
	}

	return openapi.CreateSessionResume200JSONResponse(openapi.SessionResumeResponse{
		ResumeUrl: resumeURL,
		ExpiresIn: 60,
		SessionId: sessionID,
	}), nil
}

// generateResumeToken produces a cryptographically secure opaque token.
// Returns both the raw 64-char hex string (given to the caller in the fragment)
// and its SHA-256 hex hash (stored at rest). The raw string is never persisted.
func generateResumeToken() (raw, hash string, err error) {
	b := make([]byte, rawTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("sessionresume: read random: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

// composeResumeURL builds the resume URL for the given (org, session) pair with
// the raw token embedded in the URL fragment (#rt=<rawToken>).
//
// Canonical paths:
//   - playground (orgID == "org_playground"): /playground/s/{sessionID}/resume
//   - durable org:                             /orgs/{orgID}/sessions/{sessionID}/resume
//
// Uses url.Parse so the scheme, host, and port from the configured portalURL
// are preserved regardless of whether it is https://portal.example.com or
// http://localhost:8080.
func composeResumeURL(portalURL, orgID, sessionID, rawToken string) (string, error) {
	u, err := url.Parse(portalURL)
	if err != nil {
		return "", fmt.Errorf("parse portal URL %q: %w", portalURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("portal URL %q missing scheme or host", portalURL)
	}
	u.User = nil
	u.RawQuery = ""

	if orgID == playgroundOrgID {
		u.Path = fmt.Sprintf("/playground/s/%s/resume", sessionID)
	} else {
		u.Path = fmt.Sprintf("/orgs/%s/sessions/%s/resume", orgID, sessionID)
	}

	// Embed the raw token in the fragment. url.URL.Fragment is the
	// unescaped form; url.URL.String() percent-encodes it as needed.
	u.Fragment = "rt=" + rawToken

	return u.String(), nil
}
