// Package httperr is the only place in the portal that emits an HTTP
// error response. The envelope matches docs/PROTOCOL.md > HTTP error
// contract verbatim.
package httperr

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Error is the structured error type used by all handlers.
// The JSON field names match the PROTOCOL.md envelope exactly:
//
//	{"error": "...", "message": "...", "details": {...}}
type Error struct {
	Code    string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`

	// HTTPStatus is the response code to write. Required.
	HTTPStatus int `json:"-"`

	// Wrapped is an inner error for log context (never serialized).
	Wrapped error `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Wrapped }

// Write serializes err to w using the standard envelope. Any non-*Error
// value is wrapped as ErrInternal (500). Callers should never write
// error responses except via this helper.
func Write(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = ErrInternal(err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.HTTPStatus)
	_ = json.NewEncoder(w).Encode(e)
	// Log at the level appropriate to the status.
	if e.HTTPStatus >= 500 {
		slog.ErrorContext(r.Context(), "http error",
			"code", e.Code, "status", e.HTTPStatus, "err", e.Wrapped)
	}
}

// Canonical constructors — extend per PROTOCOL.md as endpoints land.

func ErrInternal(cause error) *Error {
	return &Error{
		Code:       "internal",
		Message:    "internal server error",
		HTTPStatus: http.StatusInternalServerError,
		Wrapped:    cause,
	}
}

func ErrInvalidToken() *Error {
	return &Error{
		Code:       "auth.invalid_token",
		Message:    "invalid token",
		HTTPStatus: http.StatusUnauthorized,
	}
}

func ErrExpiredToken() *Error {
	return &Error{
		Code:       "auth.expired_token",
		Message:    "token expired",
		HTTPStatus: http.StatusUnauthorized,
	}
}

func ErrInsufficientPermission() *Error {
	return &Error{
		Code:       "auth.insufficient_permission",
		Message:    "insufficient permission",
		HTTPStatus: http.StatusForbidden,
	}
}

func ErrSessionNotFound() *Error {
	return &Error{
		Code:       "session.not_found",
		Message:    "session not found",
		HTTPStatus: http.StatusNotFound,
	}
}
