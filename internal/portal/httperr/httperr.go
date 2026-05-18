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

	// Headers, if non-empty, are written to w.Header() before the status.
	// Used by dep-failure constructors to attach Retry-After hints
	// (never serialized into the JSON body).
	Headers map[string]string `json:"-"`
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
	for k, v := range e.Headers {
		if v != "" {
			w.Header().Set(k, v)
		}
	}
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

// Dep-failure constructors. These are emitted by the WriteFromError
// translator (see translate.go) when a handler returns an error wrapped
// with one of the deperr sentinels. The 503 family carries a
// conservative Retry-After hint; git_subprocess_failed is a local
// process failure and intentionally has no Retry-After.

func ErrSMTPUnavailable(cause error) *Error {
	return &Error{
		Code:       "dep.smtp_unavailable",
		Message:    "email delivery is currently unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
		Wrapped:    cause,
		Headers:    map[string]string{"Retry-After": "5"},
	}
}

func ErrDBUnavailable(cause error) *Error {
	return &Error{
		Code:       "dep.db_unavailable",
		Message:    "database is currently unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
		Wrapped:    cause,
		Headers:    map[string]string{"Retry-After": "2"},
	}
}

func ErrOAuthProviderUnavailable(cause error) *Error {
	return &Error{
		Code:       "dep.oauth_provider_unavailable",
		Message:    "OAuth provider is currently unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
		Wrapped:    cause,
		Headers:    map[string]string{"Retry-After": "10"},
	}
}

func ErrGitSubprocessFailed(cause error) *Error {
	return &Error{
		Code:       "dep.git_subprocess_failed",
		Message:    "git subprocess failed",
		HTTPStatus: http.StatusInternalServerError,
		Wrapped:    cause,
	}
}

// ErrBadRequest is emitted when oapi-codegen's strict handler fails to
// decode a request body or path/query parameters. Replaces the default
// plain-text 400 with the standard envelope so every error response
// shares the same shape.
func ErrBadRequest(cause error) *Error {
	msg := "malformed request"
	if cause != nil {
		msg = cause.Error()
	}
	return &Error{
		Code:       "request.malformed",
		Message:    msg,
		HTTPStatus: http.StatusBadRequest,
		Wrapped:    cause,
	}
}

// ErrBodyTooLarge is emitted when the request body exceeds the configured
// limit (set by the BodyLimit middleware via http.MaxBytesReader).
// Returns 413 Request Entity Too Large with a stable error code.
func ErrBodyTooLarge() *Error {
	return &Error{
		Code:       "request.body_too_large",
		Message:    "request body exceeds the maximum allowed size",
		HTTPStatus: http.StatusRequestEntityTooLarge,
	}
}

// WriteBadRequest is a convenience wrapper around Write that constructs
// an ErrBadRequest envelope. Intended as a RequestErrorHandlerFunc on
// the oapi-codegen strict handler. If err wraps *http.MaxBytesError (set by
// the BodyLimit middleware), it replies 413 instead of 400.
func WriteBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		Write(w, r, ErrBodyTooLarge())
		return
	}
	Write(w, r, ErrBadRequest(err))
}
