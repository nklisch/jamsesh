package tokens

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/httperr"
)

type ctxKey struct{}
type rawBearerCtxKey struct{}

// BearerMiddleware returns a chi-compatible middleware that validates an
// "Authorization: Bearer <token>" header and attaches the resolved *store.Account
// to the request context. On failure it writes the appropriate PROTOCOL.md
// error envelope and halts the chain.
func BearerMiddleware(svc Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				httperr.Write(w, r, httperr.ErrInvalidToken())
				return
			}
			tok := strings.TrimPrefix(authz, prefix)
			acct, err := svc.Validate(r.Context(), tok)
			if err != nil {
				switch {
				case errors.Is(err, ErrExpiredToken):
					httperr.Write(w, r, httperr.ErrExpiredToken())
				case errors.Is(err, ErrInvalidToken), errors.Is(err, ErrRevokedToken):
					httperr.Write(w, r, httperr.ErrInvalidToken())
				default:
					// Route through the dep translator so transient DB
					// failures from svc.Validate surface as the typed
					// dep.db_unavailable 503 envelope (with Retry-After: 2)
					// rather than a generic "internal" 500. Non-DB errors
					// fall through to ErrInternal inside WriteFromError.
					httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))
				}
				return
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, acct)
			// Stash the raw bearer too so handlers like Logout can look up
			// the token row by hash without re-extracting it from the
			// Authorization header. Purely additive — existing consumers
			// of AccountFromContext are unaffected.
			// (feature-auth-signout-backend-revoke-backend)
			ctx = context.WithValue(ctx, rawBearerCtxKey{}, tok)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccountFromContext retrieves the *store.Account attached by BearerMiddleware.
// The second return value reports whether a valid account was present.
func AccountFromContext(ctx context.Context) (*store.Account, bool) {
	v, ok := ctx.Value(ctxKey{}).(*store.Account)
	return v, ok
}

// rawBearerFromContext returns the raw bearer string attached by
// BearerMiddleware. Package-private — only Handler.Logout needs it. Tests
// that bypass BearerMiddleware (e.g. via ContextWithAccount) can attach the
// raw bearer themselves via the un-exported context key — wired through
// ContextWithRawBearer.
func rawBearerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(rawBearerCtxKey{}).(string)
	return v
}

// ContextWithRawBearer attaches the raw bearer string to ctx using the
// same key BearerMiddleware uses. Tests that authenticate via
// ContextWithAccount also call this so Logout has the bearer it needs.
func ContextWithRawBearer(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, rawBearerCtxKey{}, raw)
}

// ContextWithAccount attaches the given account to ctx using the same key
// BearerMiddleware uses. Intended for tests and in-process callers that
// already authenticated by another path (e.g. MCP session auth).
//
// This is an intentionally exported helper: multiple packages' test suites
// use it to inject authenticated accounts without going through HTTP middleware.
// It is not called from production code paths.
func ContextWithAccount(ctx context.Context, acct *store.Account) context.Context {
	return context.WithValue(ctx, ctxKey{}, acct)
}
