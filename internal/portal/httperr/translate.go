package httperr

import (
	"errors"
	"net/http"

	"jamsesh/internal/portal/deperr"
)

// WriteFromError translates any handler-returned error into the
// canonical envelope:
//
//  1. *Error (via errors.As) -> write it directly. This preserves
//     today's typed-error path used by token middleware and any
//     handler that constructs an explicit envelope.
//  2. errors.Is match against a deperr.Err* sentinel -> build the
//     corresponding typed dep envelope (503 + Retry-After for the
//     upstream family, 500 for git subprocess).
//  3. Fallthrough -> ErrInternal (preserves today's "internal" 500
//     default for unanticipated errors).
//
// Wired as the strict-handler ResponseErrorHandlerFunc in
// cmd/portal/main.go.
func WriteFromError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if errors.As(err, &e) {
		Write(w, r, e)
		return
	}
	switch {
	case errors.Is(err, deperr.ErrSMTP):
		Write(w, r, ErrSMTPUnavailable(err))
	case errors.Is(err, deperr.ErrDB):
		Write(w, r, ErrDBUnavailable(err))
	case errors.Is(err, deperr.ErrOAuthProvider):
		Write(w, r, ErrOAuthProviderUnavailable(err))
	case errors.Is(err, deperr.ErrGitSubprocess):
		Write(w, r, ErrGitSubprocessFailed(err))
	default:
		Write(w, r, ErrInternal(err))
	}
}
