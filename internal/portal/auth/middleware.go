package auth

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/tokens"
)

type orgMemberCtxKey struct{}

// OrgMemberFromContext retrieves the *store.OrgMember injected by
// RequireOrgRole. The second return value reports whether one was present.
// Handlers downstream of RequireOrgRole can use this to avoid a redundant
// store lookup.
func OrgMemberFromContext(ctx context.Context) (*store.OrgMember, bool) {
	v, ok := ctx.Value(orgMemberCtxKey{}).(*store.OrgMember)
	return v, ok
}

// RequireOrgRole returns a chi middleware that verifies the authenticated
// account is a member of the org identified by the "orgID" URL parameter AND
// that their role is in the allowed set. It must be used after
// tokens.BearerMiddleware so the account is already in context.
//
// On failure it writes the canonical auth.insufficient_permission 403 envelope
// and halts the handler chain. On success the resolved OrgMember is injected
// into the context (accessible via OrgMemberFromContext).
func RequireOrgRole(s store.Store, roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acc, ok := tokens.AccountFromContext(r.Context())
			if !ok {
				// BearerMiddleware should have rejected unauthenticated requests
				// before we get here, but guard defensively.
				httperr.Write(w, r, httperr.ErrInsufficientPermission())
				return
			}

			orgID := chi.URLParam(r, "orgID")
			if orgID == "" {
				httperr.Write(w, r, httperr.ErrInsufficientPermission())
				return
			}

			member, err := s.GetOrgMember(r.Context(), store.GetOrgMemberParams{
				OrgID:     orgID,
				AccountID: acc.ID,
			})
			if err != nil {
				// Not a member (ErrNotFound) or any other error → deny.
				httperr.Write(w, r, httperr.ErrInsufficientPermission())
				return
			}

			if _, ok := allowed[member.Role]; !ok {
				httperr.Write(w, r, httperr.ErrInsufficientPermission())
				return
			}

			// Inject the resolved membership so downstream handlers don't
			// need to re-query it.
			ctx := context.WithValue(r.Context(), orgMemberCtxKey{}, &member)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
