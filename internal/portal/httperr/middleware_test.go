package httperr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/httperr"
)

func TestRecovererPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went very wrong")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/boom", nil)

	httperr.Recoverer(panicHandler).ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}

	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Error != "internal" {
		t.Errorf("want error=internal, got %q", env.Error)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("want JSON content-type, got %q", ct)
	}
}

func TestRecovererPassthrough(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ok", nil)

	httperr.Recoverer(ok).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestNotFoundHandler(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/does/not/exist", nil)

	httperr.NotFoundHandler().ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}

	var env struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Error != "route.not_found" {
		t.Errorf("want error=route.not_found, got %q", env.Error)
	}
}

func TestMethodNotAllowedHandler(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/healthz", nil)

	httperr.MethodNotAllowedHandler().ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}

	var env struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if env.Error != "route.method_not_allowed" {
		t.Errorf("want error=route.method_not_allowed, got %q", env.Error)
	}
}
