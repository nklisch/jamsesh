package httperr_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/httperr"
)

func TestWriteFromError(t *testing.T) {
	cases := []struct {
		name           string
		input          error
		wantCode       string
		wantStatus     int
		wantRetryAfter string
	}{
		{
			name:           "smtp sentinel",
			input:          deperr.WrapSMTP(errors.New("tls handshake")),
			wantCode:       "dep.smtp_unavailable",
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: "5",
		},
		{
			name:           "db sentinel",
			input:          deperr.WrapDB(errors.New("conn refused")),
			wantCode:       "dep.db_unavailable",
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: "2",
		},
		{
			name:           "oauth-provider sentinel",
			input:          deperr.WrapOAuthProvider(errors.New("502 bad gateway")),
			wantCode:       "dep.oauth_provider_unavailable",
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: "10",
		},
		{
			name:           "git-subprocess sentinel",
			input:          deperr.WrapGitSubprocess(errors.New("exit status 128")),
			wantCode:       "dep.git_subprocess_failed",
			wantStatus:     http.StatusInternalServerError,
			wantRetryAfter: "",
		},
		{
			name:           "typed *httperr.Error pass-through",
			input:          httperr.ErrInvalidToken(),
			wantCode:       "auth.invalid_token",
			wantStatus:     http.StatusUnauthorized,
			wantRetryAfter: "",
		},
		{
			name:           "default fallthrough (unanticipated error)",
			input:          errors.New("anything else"),
			wantCode:       "internal",
			wantStatus:     http.StatusInternalServerError,
			wantRetryAfter: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			httperr.WriteFromError(w, r, tc.input)

			if w.Code != tc.wantStatus {
				t.Errorf("status: want %d, got %d", tc.wantStatus, w.Code)
			}

			ct := w.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type: want application/json prefix, got %q", ct)
			}

			env := decodeEnvelope(t, w.Body.String())
			if env.Error != tc.wantCode {
				t.Errorf("error code: want %q, got %q", tc.wantCode, env.Error)
			}

			gotRetry := w.Header().Get("Retry-After")
			if gotRetry != tc.wantRetryAfter {
				t.Errorf("Retry-After: want %q, got %q", tc.wantRetryAfter, gotRetry)
			}
		})
	}
}

// TestWriteFromError_PreservesWrappedTypedError verifies that a
// *httperr.Error wrapped via fmt.Errorf("%w: ...") is still recognized
// by errors.As and not misclassified as a fallthrough.
func TestWriteFromError_PreservesWrappedTypedError(t *testing.T) {
	wrapped := errors.Join(httperr.ErrSessionNotFound(), errors.New("extra context"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	httperr.WriteFromError(w, r, wrapped)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "session.not_found" {
		t.Errorf("want session.not_found, got %q", env.Error)
	}
}

// TestWriteBadRequest emits the request.malformed envelope at 400.
func TestWriteBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	httperr.WriteBadRequest(w, r, errors.New("invalid JSON: unexpected token"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "request.malformed" {
		t.Errorf("error code: want request.malformed, got %q", env.Error)
	}
	if !strings.Contains(env.Message, "invalid JSON") {
		t.Errorf("message: want substring %q, got %q", "invalid JSON", env.Message)
	}
}
