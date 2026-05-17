package finalize

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/tokens"
)

// fetchTokenTTL is the lifetime of an ephemeral fetch-only credential.
// Locked at epic-design (5 minutes is enough for `git fetch` to complete
// across reasonable network conditions, short enough that the credential
// is uninteresting to attackers).
const fetchTokenTTL = 5 * time.Minute

// IssueFetchToken implements POST
// /api/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token.
//
// Mints a short-TTL access token bound to the caller, and returns it
// alongside a pre-composed git remote URL with the token spliced into
// the userinfo segment. Only session members may mint a token.
//
// The credential is a regular oauth_tokens row with a custom expiry —
// the basic-auth middleware on /git/... accepts it unchanged because
// TTL is per-row.
func (h *Handler) IssueFetchToken(ctx context.Context, req openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.IssueFetchToken401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, fmt.Errorf("finalize: membership check: %w", err)
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.IssueFetchToken403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.IssueFetchToken403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.IssueFetchToken404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	raw, expiresAt, err := h.tokens.IssueShortLived(ctx, acc.ID, fetchTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("finalize: issue short-lived token: %w", err)
	}

	remoteURL, err := composeFetchRemoteURL(h.portalURL, orgID, sessionID, raw)
	if err != nil {
		return nil, fmt.Errorf("finalize: compose remote URL: %w", err)
	}

	return openapi.IssueFetchToken201JSONResponse(openapi.FetchTokenResponse{
		Token:     raw,
		RemoteUrl: remoteURL,
		ExpiresAt: expiresAt.UTC(),
	}), nil
}

// composeFetchRemoteURL splices the raw token into the userinfo segment of
// the portal git smart-HTTP URL for the given session. Uses url.Parse so
// the scheme, host, and port from the configured portalURL are preserved
// regardless of whether it's https://portal.example.com or
// http://localhost:8080.
func composeFetchRemoteURL(portalURL, orgID, sessionID, rawToken string) (string, error) {
	u, err := url.Parse(portalURL)
	if err != nil {
		return "", fmt.Errorf("parse portal URL %q: %w", portalURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("portal URL %q missing scheme or host", portalURL)
	}
	u.User = url.UserPassword("x-access-token", rawToken)
	u.Path = fmt.Sprintf("/git/%s/%s.git", orgID, sessionID)
	return u.String(), nil
}
