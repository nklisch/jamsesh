// Package pusherr classifies HTTP push errors as OK, Transient, or Permanent
// using the HTTP status code and the portal's structured error envelope.
//
// Classification rules (in priority order):
//  1. No HTTP response (status 0) → Transient (network-level failure)
//  2. 2xx → OK
//  3. 5xx → Transient
//  4. 4xx with a recognised permanent error code → Permanent
//  5. Any other 4xx → Permanent (safer default for client errors)
//
// Permanent error codes are those prefixed with "push." or equal to
// "auth.invalid_token", "auth.insufficient_permission", or
// "auth.expired_token".
package pusherr

import (
	"encoding/json"
	"strings"
)

// Class is the severity class of a push error.
type Class int

const (
	// OK means the push succeeded (2xx).
	OK Class = iota
	// Transient means the push failed but may succeed if retried (network
	// errors, 5xx).
	Transient
	// Permanent means the push was rejected for a reason that will not change
	// on retry (4xx with structured error code).
	Permanent
)

// Result is the output of Classify.
type Result struct {
	Class      Class
	Code       string         // error code from JSON envelope; empty for Transient/OK with no body
	Message    string         // human-readable message from body, or empty
	Details    map[string]any // structured detail payload from body, or nil
	HTTPStatus int            // the HTTP status code; 0 for network-level errors
}

// errorEnvelope mirrors the portal's standard error response shape
// (docs/PROTOCOL.md § HTTP error contract).
type errorEnvelope struct {
	Error   string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// isPermanentCode reports whether code identifies a permanent push rejection.
func isPermanentCode(code string) bool {
	if strings.HasPrefix(code, "push.") {
		return true
	}
	switch code {
	case "auth.invalid_token", "auth.insufficient_permission", "auth.expired_token":
		return true
	}
	return false
}

// Classify returns a Result describing the error class for the given HTTP
// status and response body. Pass status=0 when there was no HTTP response
// (pure network error).
func Classify(httpStatus int, body []byte) Result {
	// Network-level failure — no HTTP response.
	if httpStatus == 0 {
		return Result{Class: Transient, HTTPStatus: 0}
	}

	// Success.
	if httpStatus >= 200 && httpStatus < 300 {
		return Result{Class: OK, HTTPStatus: httpStatus}
	}

	// Server errors are transient.
	if httpStatus >= 500 {
		r := Result{Class: Transient, HTTPStatus: httpStatus}
		// Attempt to extract a message from the body for richer diagnostics.
		if len(body) > 0 {
			var env errorEnvelope
			if err := json.Unmarshal(body, &env); err == nil {
				r.Code = env.Error
				r.Message = env.Message
				r.Details = env.Details
			}
		}
		return r
	}

	// 4xx: parse body and classify by code.
	r := Result{Class: Permanent, HTTPStatus: httpStatus}
	if len(body) > 0 {
		var env errorEnvelope
		if err := json.Unmarshal(body, &env); err == nil {
			r.Code = env.Error
			r.Message = env.Message
			r.Details = env.Details
		}
	}
	// All 4xx are permanent (the request itself is invalid or rejected).
	// If the code is a known permanent code it is already correct; otherwise
	// "Permanent" is still the right bucket (no point retrying a client error).
	return r
}
