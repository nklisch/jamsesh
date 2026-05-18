package auth_test

// Integration tests for the per-IP rate-limit middleware wired on auth
// endpoints. These tests exercise the full middleware → handler chain using a
// real ratelimit.Store and a real MagicLinkHandler backed by an in-memory
// SQLite store with a captureSender (no email I/O). They are NOT unit tests of
// the Store primitives (those live in internal/portal/ratelimit/store_test.go)
// — they exist to catch regressions in the wiring seam: per-endpoint store
// construction, chi r.With(...) mount, and the 429 + Retry-After response
// shape.
//
// Tests:
//   - TestAuthRateLimit_MagicLinkRequest_429AfterBurst
//   - TestAuthRateLimit_DifferentIPsAreIndependent
//   - TestAuthRateLimit_DisabledKnob_NeverReturns429

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/ratelimit"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildRateLimitHandler returns an http.Handler (chi router) with the
// magic-link/request endpoint guarded by a fresh ratelimit.Store configured to
// perMinute tokens per IP per minute. enabled mirrors the
// JAMSESH_AUTH_RATE_LIMIT_ENABLED config knob.
//
// Uses the same wiring pattern as cmd/portal/main.go:
//
//	r.With(mlRequestRL).Post("/api/auth/magic-link/request", apiWrapper.RequestMagicLink)
//
// Each call gets its own in-memory SQLite store and captureSender so tests
// cannot share bucket state.
func buildRateLimitHandler(t *testing.T, perMinute int, enabled bool) (http.Handler, *captureSender) {
	t.Helper()

	s := openStore(t) // in-memory SQLite; cleaned up via t.Cleanup
	sender := &captureSender{}
	tokenSvc := tokens.New(s)
	magicLink := auth.NewMagicLinkHandler(s, tokenSvc, sender, "https://portal.example.com")

	// Mirror cmd/portal/main.go's strict-handler wiring.
	fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: magicLink}
	strictAPI := openapi.NewStrictHandlerWithOptions(fullHandler, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	// Wire the rate-limit middleware, mirroring main.go's per-endpoint pattern.
	mlRequestRL := ratelimit.NewStore(ratelimit.Config{PerMinute: perMinute}).Middleware(enabled)

	r := chi.NewRouter()
	r.With(mlRequestRL).Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
	return r, sender
}

// fireMagicLinkRequest fires a single POST /api/auth/magic-link/request
// directly against handler using httptest.NewRecorder. remoteAddr is set on
// the request so the rate-limit middleware keys the bucket by IP.
func fireMagicLinkRequest(handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"email": "test@example.com"})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/magic-link/request",
		bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// ---------------------------------------------------------------------------
// TestAuthRateLimit_MagicLinkRequest_429AfterBurst
//
// Verifies that:
//   - Requests 1…perMinute succeed (204 from the magic-link handler).
//   - Request perMinute+1 returns 429 with Retry-After > 0 and the
//     {"error": "rate_limited", "message": "..."} envelope.
// ---------------------------------------------------------------------------

func TestAuthRateLimit_MagicLinkRequest_429AfterBurst(t *testing.T) {
	const perMinute = 3

	h, _ := buildRateLimitHandler(t, perMinute, true)

	const ip = "203.0.113.1:12345"

	// Requests 1–3 must pass (burst == perMinute).
	for i := range perMinute {
		w := fireMagicLinkRequest(h, ip)
		if w.Code != http.StatusNoContent {
			t.Errorf("request %d: want 204, got %d", i+1, w.Code)
		}
	}

	// Request 4 must be rate-limited.
	w := fireMagicLinkRequest(h, ip)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d: want 429, got %d", perMinute+1, w.Code)
	}

	// Retry-After must be present and a positive integer.
	ra := w.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("429 response missing Retry-After header")
	}
	secs, err := strconv.Atoi(ra)
	if err != nil || secs <= 0 {
		t.Fatalf("Retry-After must be a positive integer; got %q", ra)
	}

	// JSON body must match {"error": "rate_limited", "message": "..."}.
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if body["error"] != "rate_limited" {
		t.Errorf("error field: want %q, got %q", "rate_limited", body["error"])
	}
	if body["message"] == "" {
		t.Error("message field must be non-empty in 429 body")
	}
}

// ---------------------------------------------------------------------------
// TestAuthRateLimit_DifferentIPsAreIndependent
//
// Exhausts the rate-limit bucket for IP_A, then fires one request from IP_B
// and asserts it succeeds — per-IP buckets must be independent.
// ---------------------------------------------------------------------------

func TestAuthRateLimit_DifferentIPsAreIndependent(t *testing.T) {
	const perMinute = 3

	h, _ := buildRateLimitHandler(t, perMinute, true)

	const ipA = "203.0.113.10:1111"
	const ipB = "203.0.113.20:2222"

	// Exhaust IP_A's burst.
	for range perMinute {
		fireMagicLinkRequest(h, ipA)
	}

	// Confirm IP_A is now rate-limited.
	wA := fireMagicLinkRequest(h, ipA)
	if wA.Code != http.StatusTooManyRequests {
		t.Fatalf("IP_A: want 429 after burst, got %d", wA.Code)
	}

	// IP_B must not be affected by IP_A's exhausted bucket.
	wB := fireMagicLinkRequest(h, ipB)
	if wB.Code != http.StatusNoContent {
		t.Errorf("IP_B: want 204 (independent bucket), got %d", wB.Code)
	}
}

// ---------------------------------------------------------------------------
// TestAuthRateLimit_DisabledKnob_NeverReturns429
//
// With the rate-limit disabled (mirrors JAMSESH_AUTH_RATE_LIMIT_ENABLED=false),
// all requests from the same IP must pass through, even well past the burst.
// ---------------------------------------------------------------------------

func TestAuthRateLimit_DisabledKnob_NeverReturns429(t *testing.T) {
	const perMinute = 3
	const totalRequests = 20

	// enabled=false: Middleware returns the bare next handler with no token check.
	h, _ := buildRateLimitHandler(t, perMinute, false)

	const ip = "203.0.113.99:9999"

	for i := range totalRequests {
		w := fireMagicLinkRequest(h, ip)
		if w.Code == http.StatusTooManyRequests {
			t.Errorf("request %d: got unexpected 429 (rate limiting should be disabled)", i+1)
		}
	}
}
