package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/internal/portal/router"
)

// metricsHandler is a minimal http.Handler that returns 200 with a realistic
// Prometheus content-type header. Used as the MetricsHandler stub.
var metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("# HELP go_goroutines Number of goroutines.\n"))
	_, _ = w.Write([]byte("# TYPE go_goroutines gauge\n"))
	_, _ = w.Write([]byte("go_goroutines 1\n"))
})

// TestMetricsUnmounted verifies that when MetricsToken is empty (the default),
// /metrics is not registered and the path falls through to the 404 handler.
// This is the JAMSESH_METRICS_TOKEN unset case — the endpoint must not exist at all.
func TestMetricsUnmounted(t *testing.T) {
	h := router.New(router.Deps{
		MetricsHandler: metricsHandler,
		MetricsToken:   "", // empty → route not mounted
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 when MetricsToken is empty, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "route.not_found" {
		t.Errorf("want error=route.not_found, got %q", env.Error)
	}
}

// TestMetricsUnmountedNilHandler verifies that when MetricsHandler is nil,
// /metrics is also not registered regardless of MetricsToken.
func TestMetricsUnmountedNilHandler(t *testing.T) {
	h := router.New(router.Deps{
		MetricsHandler: nil,
		MetricsToken:   "secret",
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 when MetricsHandler is nil, got %d", w.Code)
	}
}

// TestMetricsBearerAuth verifies the full auth contract when MetricsToken is set:
//   - no Authorization header → 401
//   - wrong bearer token → 401
//   - correct bearer token → 200 with Prometheus body
func TestMetricsBearerAuth(t *testing.T) {
	const token = "test-secret"
	h := router.New(router.Deps{
		MetricsHandler: metricsHandler,
		MetricsToken:   token,
	})

	t.Run("no_auth_header", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		h.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401 with no auth header, got %d", w.Code)
		}
		env := decodeEnvelope(t, w.Body.String())
		if env.Error != "auth.invalid_token" {
			t.Errorf("want error=auth.invalid_token, got %q", env.Error)
		}
	})

	t.Run("wrong_bearer_token", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		r.Header.Set("Authorization", "Bearer wrong-token")
		h.ServeHTTP(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401 with wrong bearer token, got %d", w.Code)
		}
		env := decodeEnvelope(t, w.Body.String())
		if env.Error != "auth.invalid_token" {
			t.Errorf("want error=auth.invalid_token, got %q", env.Error)
		}
	})

	t.Run("correct_bearer_token", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200 with correct bearer token, got %d\nbody: %s", w.Code, w.Body.String())
		}
		ct := w.Header().Get("Content-Type")
		if ct == "" {
			t.Errorf("want Content-Type header from metrics handler, got empty")
		}
	})
}

// TestMetricsBearerCaseSensitive verifies that bearer token comparison is
// case-sensitive: "Secret" must not pass when the configured token is "secret".
func TestMetricsBearerCaseSensitive(t *testing.T) {
	h := router.New(router.Deps{
		MetricsHandler: metricsHandler,
		MetricsToken:   "secret",
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "Bearer Secret") // wrong case
	h.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for wrong-case token, got %d", w.Code)
	}
}
