package githttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

type accountCtxKey struct{}

// AccountFromContext retrieves the *store.Account attached by basicAuth.
// The second return value reports whether a valid account was present.
func AccountFromContext(ctx context.Context) (*store.Account, bool) {
	v, ok := ctx.Value(accountCtxKey{}).(*store.Account)
	return v, ok
}

// basicAuth parses an "Authorization: Basic <b64>" header, decodes the
// username:password pair, and validates the password as a portal token via
// tokens.BasicAuthValidator. The username is ignored (git uses any string).
//
// On success the resolved *store.Account is attached to the request context
// and the chain continues.
// On failure a 401 with WWW-Authenticate: Basic realm="jamsesh" is returned.
func (h *Handler) basicAuth(next http.Handler) http.Handler {
	validate := tokens.BasicAuthValidator(h.Tokens)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			writeBasicUnauthorized(w)
			return
		}

		acct, err := validate(r.Context(), user, pass)
		if err != nil {
			switch {
			case errors.Is(err, tokens.ErrInvalidToken),
				errors.Is(err, tokens.ErrExpiredToken),
				errors.Is(err, tokens.ErrRevokedToken):
				writeBasicUnauthorized(w)
			default:
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}

		ctx := context.WithValue(r.Context(), accountCtxKey{}, acct)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireSessionMember verifies that the authenticated account is a member of
// the session identified by the {orgID} and {sessionID} URL parameters.
//
// A missing membership returns 401 rather than 403 to avoid leaking whether
// the session exists to non-members. Any unexpected store error returns 500.
func (h *Handler) requireSessionMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct, ok := AccountFromContext(r.Context())
		if !ok {
			// basicAuth should have rejected unauthenticated requests, but guard
			// defensively.
			writeBasicUnauthorized(w)
			return
		}

		orgID := chi.URLParam(r, "orgID")
		sessionID := chi.URLParam(r, "sessionID")

		_, err := h.Store.GetSessionMember(r.Context(), store.GetSessionMemberParams{
			OrgID:     orgID,
			SessionID: sessionID,
			AccountID: acct.ID,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Don't reveal session existence — return 401, not 403/404.
				writeBasicUnauthorized(w)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checkArchived looks up the session in the archived_sessions table. If the
// session has been archived it returns 410 Gone with a JSON body produced by
// storage.StubResponse. Otherwise the chain continues.
func (h *Handler) checkArchived(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := chi.URLParam(r, "orgID")
		sessionID := chi.URLParam(r, "sessionID")

		rec, err := h.Storage.LookupArchived(r.Context(), orgID, sessionID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Not archived — continue.
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Session is archived: return 410 Gone with the stub JSON body.
		stub := h.Storage.StubResponse(rec)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(stub)
	})
}

// writeBasicUnauthorized writes a 401 Unauthorized response with the
// WWW-Authenticate header required by RFC 7617 so that git prompts for
// credentials on the next attempt.
func writeBasicUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="jamsesh"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
