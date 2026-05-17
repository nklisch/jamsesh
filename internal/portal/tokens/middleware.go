package tokens

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/httperr"
)

type ctxKey struct{}

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
					httperr.Write(w, r, httperr.ErrInternal(err))
				}
				return
			}
			ctx := context.WithValue(r.Context(), ctxKey{}, acct)
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

// ContextWithAccount attaches the given account to ctx using the same key
// BearerMiddleware uses. Intended for tests and in-process callers that
// already authenticated by another path (e.g. MCP session auth).
func ContextWithAccount(ctx context.Context, acct *store.Account) context.Context {
	return context.WithValue(ctx, ctxKey{}, acct)
}
