package tokens

import (
	"context"

	"jamsesh/internal/db/store"
)

// BasicAuthValidator returns a function suitable for plugging into the git
// smart-HTTP handler's per-request Basic-auth check. The username is ignored
// (git uses any string as the username for HTTP Basic); the password is the
// token. It surfaces the same sentinel errors (ErrInvalidToken, ErrExpiredToken,
// ErrRevokedToken) so callers can switch on them identically to BearerMiddleware.
func BasicAuthValidator(svc Service) func(ctx context.Context, user, pass string) (*store.Account, error) {
	return func(ctx context.Context, _user, pass string) (*store.Account, error) {
		return svc.Validate(ctx, pass)
	}
}
