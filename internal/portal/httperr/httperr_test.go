package httperr_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/httperr"
)

// envelope mirrors the JSON shape from PROTOCOL.md for decoding test responses.
type envelope struct {
	Error   string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func decodeEnvelope(t *testing.T, body string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &e); err != nil {
		t.Fatalf("failed to decode envelope: %v\nbody: %s", err, body)
	}
	return e
}

func TestErrorJSONShape(t *testing.T) {
	e := &httperr.Error{
		Code:       "test.code",
		Message:    "test message",
		HTTPStatus: http.StatusBadRequest,
	}

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Must decode to exactly the protocol envelope fields.
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := got["error"]; !ok {
		t.Error("JSON missing 'error' field")
	}
	if _, ok := got["message"]; !ok {
		t.Error("JSON missing 'message' field")
	}
	// details omitted when nil
	if _, ok := got["details"]; ok {
		t.Error("JSON must NOT include 'details' when nil")
	}
	// internal fields must not leak
	if _, ok := got["HTTPStatus"]; ok {
		t.Error("JSON must NOT include 'HTTPStatus'")
	}
	if _, ok := got["Wrapped"]; ok {
		t.Error("JSON must NOT include 'Wrapped'")
	}
}

func TestErrorJSONWithDetails(t *testing.T) {
	e := &httperr.Error{
		Code:       "push.scope_violation",
		Message:    "scope violation",
		Details:    map[string]any{"paths": []string{"a/b.go"}},
		HTTPStatus: http.StatusForbidden,
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["details"]; !ok {
		t.Error("JSON must include 'details' when non-nil")
	}
}

func TestErrorStringAndUnwrap(t *testing.T) {
	cause := errors.New("db down")
	e := httperr.ErrInternal(cause)

	if e.Error() == "" {
		t.Error("Error() must not be empty")
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is should traverse Unwrap to cause")
	}
}

func TestWriteNonHTTPError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	httperr.Write(w, r, errors.New("something exploded"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "internal" {
		t.Errorf("want error=internal, got %q", env.Error)
	}
}


func TestCanonicalConstructors(t *testing.T) {
	tests := []struct {
		name       string
		err        *httperr.Error
		wantCode   string
		wantStatus int
	}{
		{"ErrInvalidToken", httperr.ErrInvalidToken(), "auth.invalid_token", http.StatusUnauthorized},
		{"ErrExpiredToken", httperr.ErrExpiredToken(), "auth.expired_token", http.StatusUnauthorized},
		{"ErrInsufficientPermission", httperr.ErrInsufficientPermission(), "auth.insufficient_permission", http.StatusForbidden},
		{"ErrInternal", httperr.ErrInternal(nil), "internal", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code: want %q, got %q", tt.wantCode, tt.err.Code)
			}
			if tt.err.HTTPStatus != tt.wantStatus {
				t.Errorf("HTTPStatus: want %d, got %d", tt.wantStatus, tt.err.HTTPStatus)
			}
		})
	}
}

func TestContentTypeHeader(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	httperr.Write(w, r, httperr.ErrInvalidToken())
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("want application/json content-type, got %q", ct)
	}
}
