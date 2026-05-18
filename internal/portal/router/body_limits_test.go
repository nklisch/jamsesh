package router_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/router"
)

// readAllOrReject is a small helper that mimics what the oapi-codegen
// strict-server RequestErrorHandlerFunc does: it reads the body, detects
// *http.MaxBytesError, and replies 413. This is the expected production
// path — the BodyLimit middleware sets the cap and the decode layer detects
// the overflow.
func readAllOrReject(w http.ResponseWriter, r *http.Request) bool {
	if _, err := io.ReadAll(r.Body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	return true
}

// TestBodyLimitMiddleware verifies that BodyLimit wraps r.Body with
// http.MaxBytesReader and that exceeding the cap returns a *http.MaxBytesError
// which callers translate to 413.
func TestBodyLimitMiddleware(t *testing.T) {
	const limit int64 = 16 // tiny cap so the test body is small

	// A handler that reads the entire body.
	// When MaxBytesReader limit is exceeded, io.ReadAll returns *http.MaxBytesError,
	// and the handler must detect it and reply 413.
	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !readAllOrReject(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := router.BodyLimit(limit)(echoHandler)

	t.Run("body within limit returns 200", func(t *testing.T) {
		body := strings.NewReader("small")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/anything", body)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("want 200, got %d", w.Code)
		}
	})

	t.Run("body exceeding limit returns 413", func(t *testing.T) {
		// 17 bytes — one over the 16-byte cap.
		body := strings.NewReader("12345678901234567")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/anything", body)
		wrapped.ServeHTTP(w, r)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("want 413, got %d", w.Code)
		}
	})
}

// TestAPIBodyLimitApplied verifies that the 1 MiB body cap is active on /api/*
// routes when wired through the full router, and that the overflow is detectable
// as *http.MaxBytesError (the error type the strict-server RequestErrorHandlerFunc
// translates to 413).
func TestAPIBodyLimitApplied(t *testing.T) {
	// Over-limit body: 1 MiB + 1 byte.
	overLimit := strings.Repeat("x", (1<<20)+1)

	h := router.New(router.Deps{
		MountAPI: func(r chi.Router) {
			r.Post("/probe", func(w http.ResponseWriter, r *http.Request) {
				if !readAllOrReject(w, r) {
					return
				}
				w.WriteHeader(http.StatusOK)
			})
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/probe", strings.NewReader(overLimit))
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413 on /api POST exceeding 1 MiB, got %d", w.Code)
	}
}
