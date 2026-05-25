// Package handlerauth provides composable auth-guard helpers for portal HTTP
// handlers. It extracts account-from-context and org/session membership checks
// into one place so individual handlers don't hand-roll repeated 401/403 blocks.
//
// Usage pattern:
//
//	acc, fail, ok := handlerauth.RequireAccount(ctx)
//	if !ok {
//	    return myOp401JSONResponse{UnauthorizedJSONResponse: fail.Unauthorized}, nil
//	}
//
//	acc, member, fail, ok := handlerauth.RequireOrgMember(ctx, s, orgID)
//	if !ok {
//	    if fail.Err != nil { return nil, fmt.Errorf("pkg: %w", fail.Err) }
//	    if fail.Status == 401 {
//	        return myOp401JSONResponse{UnauthorizedJSONResponse: fail.Unauthorized}, nil
//	    }
//	    return myOp403JSONResponse{ForbiddenJSONResponse: fail.Forbidden}, nil
//	}
package handlerauth

import (
	"context"
	"errors"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// orgMemberStore is the minimal store interface required by RequireOrgMember.
type orgMemberStore interface {
	store.OrgMemberStore
}

// sessionMemberStore is the minimal store interface required by RequireSessionMember.
type sessionMemberStore interface {
	store.SessionMemberStore
}

// AuthFail carries the typed failure payload and HTTP status hint.
//
// Status is 401, 403, or 500. For 401 the Unauthorized field is populated.
// For 403 the Forbidden field is populated. For 500 the Err field is populated
// and Status==500 signals an internal error — callers should surface it as a
// 500 via the strict-server error return path (return nil, fmt.Errorf(...)).
//
// The dual-field approach (rather than interface{}) keeps call sites type-safe
// and avoids any type assertion at the call site.
type AuthFail struct {
	Status       int
	Unauthorized openapi.UnauthorizedJSONResponse
	Forbidden    openapi.ForbiddenJSONResponse
	// Err is set when Status == 500 (unexpected store error). Callers should
	// wrap and return this as the second return value of the handler function so
	// the strict server converts it to a 500 response.
	Err error
}

// RequireAccount extracts the authenticated account from ctx.
// Returns ok=false with AuthFail{Status:401} when no account is present.
func RequireAccount(ctx context.Context) (*store.Account, AuthFail, bool) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return nil, AuthFail{
			Status: 401,
			Unauthorized: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, false
	}
	return acc, AuthFail{}, true
}

// RequireOrgMember calls RequireAccount, then verifies that the account is a
// member of orgID. Returns ok=false with an appropriate AuthFail on failure:
//   - Status 401 when no account is in ctx
//   - Status 403 when the account is not a member of the org
//   - Status 500 (Err populated) when the store returns an unexpected error
func RequireOrgMember(ctx context.Context, s orgMemberStore, orgID string) (*store.Account, store.OrgMember, AuthFail, bool) {
	acc, fail, ok := RequireAccount(ctx)
	if !ok {
		return nil, store.OrgMember{}, fail, false
	}

	member, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.OrgMember{}, AuthFail{
				Status: 403,
				Forbidden: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, false
		}
		return nil, store.OrgMember{}, AuthFail{
			Status: 500,
			Err:    err,
		}, false
	}

	return acc, member, AuthFail{}, true
}

// RequireAnonymousSessionMember is an alias for RequireSessionMember that
// documents the playground-specific contract: the caller's anonymous bearer
// MUST have been issued for the requested session_id. Because anonymous
// bearers are minted per-session by IssueAnonymousSessionBearer (creating a
// fresh anon account each time), a session-member check is equivalent to
// "bearer was issued for this session" — the underlying anon account row
// only exists because the bearer was issued, and the account is added as a
// session member in the same transaction.
//
// Use this in place of `RequireAccount + GetSessionMember` on playground
// endpoints. It composes the same checks but the named helper documents
// the cross-session-bearer-reuse defense (story:
// gate-security-anon-bearer-validate-no-session-binding).
//
// Durable-session callers should continue to use RequireSessionMember
// directly; the distinction matters only for documentation.
func RequireAnonymousSessionMember(ctx context.Context, s sessionMemberStore, orgID, sessionID string) (*store.Account, store.SessionMember, AuthFail, bool) {
	return RequireSessionMember(ctx, s, orgID, sessionID)
}

// RequireSessionMember verifies that the authenticated account is a member of
// the given session (identified by orgID + sessionID). It does NOT check org
// membership — callers that need an org-membership gate should use
// RequireOrgMember instead.
//
// Returns ok=false with an appropriate AuthFail on failure:
//   - Status 401 when no account is in ctx
//   - Status 403 when the account is not a member of the session
//   - Status 500 (Err populated) when the store returns an unexpected error
func RequireSessionMember(ctx context.Context, s sessionMemberStore, orgID, sessionID string) (*store.Account, store.SessionMember, AuthFail, bool) {
	acc, fail, ok := RequireAccount(ctx)
	if !ok {
		return nil, store.SessionMember{}, fail, false
	}

	member, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.SessionMember{}, AuthFail{
				Status: 403,
				Forbidden: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, false
		}
		return nil, store.SessionMember{}, AuthFail{
			Status: 500,
			Err:    err,
		}, false
	}

	return acc, member, AuthFail{}, true
}
