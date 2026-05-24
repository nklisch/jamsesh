package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/router"
)

// stubHandler is a minimal http.Handler that writes 200 OK.
var stubHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// assertBaselineHeaders checks that the five mandatory baseline security
// headers are present with the expected canonical values.
func assertBaselineHeaders(t *testing.T, h http.Header, label string) {
	t.Helper()

	checkHeader := func(name, want string) {
		t.Helper()
		got := h.Get(name)
		if got != want {
			t.Errorf("%s: header %q: want %q, got %q", label, name, want, got)
		}
	}

	checkHeader("X-Content-Type-Options", "nosniff")
	checkHeader("X-Frame-Options", "DENY")
	checkHeader("Referrer-Policy", "no-referrer")
}

// TestSecurityHeaders_Middleware exercises SecurityHeaders directly.
func TestSecurityHeaders_Middleware(t *testing.T) {
	t.Run("baseline headers present when HSTS disabled", func(t *testing.T) {
		mw := router.SecurityHeaders(router.SecurityHeadersOptions{EnableHSTS: false})
		h := mw(stubHandler)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(w, r)

		assertBaselineHeaders(t, w.Header(), "HSTS-off")

		// CSP must be set (non-empty).
		if csp := w.Header().Get("Content-Security-Policy"); csp == "" {
			t.Error("Content-Security-Policy should be set but was empty")
		}

		// HSTS must be absent.
		if hsts := w.Header().Get("Strict-Transport-Security"); hsts != "" {
			t.Errorf("Strict-Transport-Security should be absent when EnableHSTS=false, got %q", hsts)
		}
	})

	t.Run("HSTS enabled sets correct HSTS header", func(t *testing.T) {
		mw := router.SecurityHeaders(router.SecurityHeadersOptions{EnableHSTS: true})
		h := mw(stubHandler)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(w, r)

		assertBaselineHeaders(t, w.Header(), "HSTS-on")

		want := "max-age=31536000; includeSubDomains"
		if got := w.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("Strict-Transport-Security: want %q, got %q", want, got)
		}
	})

	t.Run("default CSP contains critical directives", func(t *testing.T) {
		mw := router.SecurityHeaders(router.SecurityHeadersOptions{})
		h := mw(stubHandler)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(w, r)

		csp := w.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatal("Content-Security-Policy header was empty")
		}

		// Spot-check critical directives instead of asserting the full literal
		// string (which would make the test brittle to whitespace changes).
		for _, want := range []string{
			"default-src 'self'",
			"frame-ancestors 'none'",
			"object-src 'none'",
			"base-uri 'none'",
		} {
			if !strings.Contains(csp, want) {
				t.Errorf("CSP missing directive %q; full CSP: %s", want, csp)
			}
		}
	})

	t.Run("caller can override CSP", func(t *testing.T) {
		custom := "default-src 'self' https://cdn.example.com"
		mw := router.SecurityHeaders(router.SecurityHeadersOptions{CSP: custom})
		h := mw(stubHandler)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(w, r)

		if got := w.Header().Get("Content-Security-Policy"); got != custom {
			t.Errorf("Content-Security-Policy: want custom %q, got %q", custom, got)
		}
	})
}

// TestSecurityHeaders_RouterIntegration verifies that the full router emits
// security headers on /healthz under various TLSMode / TrustProxyHeaders
// configurations.
func TestSecurityHeaders_RouterIntegration(t *testing.T) {
	// assertHasSecurity is a helper that hits /healthz and checks headers.
	assertHasSecurity := func(t *testing.T, d router.Deps, wantHSTS bool) {
		t.Helper()
		h := router.New(d)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}

		assertBaselineHeaders(t, w.Header(), t.Name())

		csp := w.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Error("Content-Security-Policy must be set by the router")
		}

		hsts := w.Header().Get("Strict-Transport-Security")
		if wantHSTS && hsts == "" {
			t.Errorf("Strict-Transport-Security should be set for %+v", d)
		}
		if !wantHSTS && hsts != "" {
			t.Errorf("Strict-Transport-Security should be absent for %+v, got %q", d, hsts)
		}
	}

	t.Run("TLSMode empty no HSTS", func(t *testing.T) {
		assertHasSecurity(t, router.Deps{Security: router.Security{TLSMode: ""}}, false)
	})

	t.Run("TLSMode native enables HSTS", func(t *testing.T) {
		assertHasSecurity(t, router.Deps{Security: router.Security{TLSMode: "native"}}, true)
	})

	t.Run("TLSMode behind_proxy with TrustProxyHeaders enables HSTS", func(t *testing.T) {
		assertHasSecurity(t, router.Deps{Security: router.Security{TLSMode: "behind_proxy", TrustProxyHeaders: true}}, true)
	})

	t.Run("TLSMode behind_proxy without TrustProxyHeaders no HSTS", func(t *testing.T) {
		assertHasSecurity(t, router.Deps{Security: router.Security{TLSMode: "behind_proxy", TrustProxyHeaders: false}}, false)
	})
}
