package httperr

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer converts panics to the JSON envelope. Replaces chi's default
// text/plain recoverer so every error path — including panics — returns the
// standard PROTOCOL.md envelope.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"recover", fmt.Sprint(rec),
					"stack", string(debug.Stack()))
				Write(w, r, &Error{
					Code:       "internal",
					Message:    "internal server error",
					HTTPStatus: http.StatusInternalServerError,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// NotFoundHandler returns the JSON envelope for unknown routes.
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Write(w, r, &Error{
			Code:       "route.not_found",
			Message:    "no route matches",
			HTTPStatus: http.StatusNotFound,
		})
	})
}

// MethodNotAllowedHandler returns the JSON envelope for method mismatch.
func MethodNotAllowedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Write(w, r, &Error{
			Code:       "route.method_not_allowed",
			Message:    "method not allowed for route",
			HTTPStatus: http.StatusMethodNotAllowed,
		})
	})
}
