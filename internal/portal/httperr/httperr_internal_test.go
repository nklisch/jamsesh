package httperr

// Internal tests for package-private constructors.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// internalEnvelope mirrors the JSON shape for decoding internal test responses.
type internalEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func decodeInternalEnvelope(t *testing.T, body string) internalEnvelope {
	t.Helper()
	var e internalEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &e); err != nil {
		t.Fatalf("failed to decode envelope: %v\nbody: %s", err, body)
	}
	return e
}

func TestWriteHTTPError_SessionNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	Write(w, r, errSessionNotFound())

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	env := decodeInternalEnvelope(t, w.Body.String())
	if env.Error != "session.not_found" {
		t.Errorf("want session.not_found, got %q", env.Error)
	}
}

func TestErrSessionNotFound_Constructor(t *testing.T) {
	err := errSessionNotFound()
	if err.Code != "session.not_found" {
		t.Errorf("Code: want session.not_found, got %q", err.Code)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus: want 404, got %d", err.HTTPStatus)
	}
}

func TestWriteFromError_PreservesWrappedTypedError_SessionNotFound(t *testing.T) {
	wrapped := errors.Join(errSessionNotFound(), errors.New("extra context"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	WriteFromError(w, r, wrapped)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	env := decodeInternalEnvelope(t, w.Body.String())
	if env.Error != "session.not_found" {
		t.Errorf("want session.not_found, got %q", env.Error)
	}
}
