package tokens

import (
	"context"
	"errors"

	"jamsesh/internal/api/openapi"
)

// Handler implements the openapi.StrictServerInterface methods for the token
// endpoints: POST /api/auth/refresh and POST /api/auth/revoke.
//
// Wiring note: the two methods have different security requirements so they are
// NOT mounted via a single HandlerFromMux call. Instead cmd/portal/main.go uses
// two r.Route groups — one public (refresh) and one behind BearerMiddleware
// (revoke) — and calls HandlerFromMux on each group. This keeps the middleware
// split cleanly at the router level rather than inside the handler.
type Handler struct {
	svc Service
}

// NewHandler returns a Handler backed by svc.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Handler satisfies the RefreshToken and RevokeToken methods of
// openapi.StrictServerInterface. The full interface is satisfied by a
// combined handler in cmd/portal/main.go that composes this handler with
// additional feature handlers (e.g. magic-link).

// RefreshToken implements POST /api/auth/refresh.
// This endpoint is PUBLIC — the refresh token in the request body is the
// credential. No Bearer middleware is applied upstream.
// auth flow: body-based refresh token (not bearer); no handlerauth migration
// needed or possible. see refactor-handler-auth-guards-accounts-tokens
func (h *Handler) RefreshToken(ctx context.Context, req openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	pair, err := h.svc.Refresh(ctx, req.Body.RefreshToken)
	if err != nil {
		return mapToRefreshError(err), nil
	}
	return openapi.RefreshToken200JSONResponse{
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt,
		RefreshExpiresAt: pair.RefreshExpiresAt,
	}, nil
}

// RevokeToken implements POST /api/auth/revoke.
// This endpoint requires a valid Bearer token; BearerMiddleware is applied
// upstream and attaches the *store.Account to the request context.
// revokeAll: true revokes every token for the authenticated account.
//
// auth flow: handlerauth cannot be used here because handlerauth imports this
// package (tokens) for AccountFromContext, creating a cycle. AccountFromContext
// is called directly instead. see refactor-handler-auth-guards-accounts-tokens
func (h *Handler) RevokeToken(ctx context.Context, req openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	// Belt-and-suspenders: BearerMiddleware should have blocked unauthenticated
	// requests before they reach here, but guard explicitly.
	acct, ok := AccountFromContext(ctx)
	if !ok {
		return openapi.RevokeToken401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	revokeAll := req.Body.RevokeAll
	if err := h.svc.Revoke(ctx, acct.ID, req.Body.Token, revokeAll); err != nil {
		if errors.Is(err, ErrForbidden) {
			return openapi.RevokeToken403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.forbidden",
					Message: "token does not belong to the authenticated account",
				},
			}, nil
		}
		return openapi.RevokeToken401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}
	return openapi.RevokeToken204Response{}, nil
}

// Logout implements POST /api/auth/logout. Revokes ALL tokens for the
// authenticated account (logout-everywhere semantics) using the bearer
// presented in the Authorization header. No request body required.
//
// (feature-auth-signout-backend-revoke-backend)
//
// auth flow: same import-cycle constraint as RevokeToken — AccountFromContext
// is called directly rather than via handlerauth.
func (h *Handler) Logout(ctx context.Context, _ openapi.LogoutRequestObject) (openapi.LogoutResponseObject, error) {
	acct, ok := AccountFromContext(ctx)
	if !ok {
		return openapi.Logout401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}
	rawToken := rawBearerFromContext(ctx)
	if rawToken == "" {
		// BearerMiddleware always populates this when an account is in ctx;
		// the explicit nil check is belt-and-suspenders for direct context
		// injection paths that forget ContextWithRawBearer.
		return openapi.Logout401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}
	if err := h.svc.Revoke(ctx, acct.ID, rawToken, true); err != nil {
		// ErrForbidden cannot occur in practice: we pass acct.ID as both
		// caller and the token's owner (BearerMiddleware fetched acct via
		// the same token's hash). Map any error to 401 to avoid leaking
		// internals.
		return openapi.Logout401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}
	return openapi.Logout204Response{}, nil
}

// mapToRefreshError converts token sentinel errors to the appropriate 401 response.
func mapToRefreshError(err error) openapi.RefreshTokenResponseObject {
	var code, msg string
	switch {
	case errors.Is(err, ErrExpiredToken):
		code = "auth.expired_token"
		msg = "token expired"
	case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrRevokedToken):
		code = "auth.invalid_token"
		msg = "invalid token"
	default:
		// Unexpected errors surface as invalid token (don't leak internal details).
		code = "auth.invalid_token"
		msg = "invalid token"
	}
	return openapi.RefreshToken401JSONResponse{
		UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
			Error:   code,
			Message: msg,
		},
	}
}
