package router_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/router"
)

// envelope is the PROTOCOL.md JSON error shape.
type envelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func decodeEnvelope(t *testing.T, body string) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &e); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, body)
	}
	return e
}

func TestHealthz(t *testing.T) {
	h := router.New(router.Deps{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("want status=ok, got %q", body["status"])
	}
	if body["version"] == "" {
		t.Errorf("want non-empty version field in /healthz response")
	}
}

// ---------------------------------------------------------------------------
// bug-csp-report-endpoint-not-wired
// ---------------------------------------------------------------------------

func TestCSPReport_ValidJSON_Returns204(t *testing.T) {
	// Capture slog output to verify the csp_violation key is logged.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := router.New(router.Deps{})
	body := strings.NewReader(`{"csp-report":{"violated-directive":"script-src 'self'","blocked-uri":"https://evil.example.com"}}`)
	r := httptest.NewRequest(http.MethodPost, "/_csp-report", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", w.Code)
	}
	// Body must be empty (204 spec).
	if w.Body.Len() != 0 {
		t.Errorf("want empty body, got %q", w.Body.String())
	}
	// Log line must include csp_violation key.
	if !strings.Contains(buf.String(), `"msg":"csp_violation"`) {
		t.Errorf("log line missing csp_violation msg: %s", buf.String())
	}
}

func TestCSPReport_MalformedBody_Still204(t *testing.T) {
	h := router.New(router.Deps{})
	r := httptest.NewRequest(http.MethodPost, "/_csp-report", strings.NewReader(`{bad json`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("malformed body: want 204 (no 400), got %d", w.Code)
	}
}

func TestCSPReport_WrongMethodReturns405(t *testing.T) {
	h := router.New(router.Deps{})
	r := httptest.NewRequest(http.MethodGet, "/_csp-report", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
	// chi's MethodNotAllowed renders the JSON envelope.
	got := decodeEnvelope(t, w.Body.String())
	if got.Error == "" {
		t.Errorf("want non-empty error code, got %+v", got)
	}
}

func TestCSPReport_NoAuthHeader_Returns204(t *testing.T) {
	// The endpoint must accept unauthenticated POSTs — browsers send CSP
	// reports without credentials.
	h := router.New(router.Deps{})
	r := httptest.NewRequest(http.MethodPost, "/_csp-report", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("want 204 with no auth header, got %d", w.Code)
	}
}

func TestCSPReport_OversizedBody_Returns204(t *testing.T) {
	// 128 KiB > 64 KiB MaxBytesReader cap; the decoder returns an error and
	// the handler returns 204 (no 4xx). Body content is not echoed.
	h := router.New(router.Deps{})
	huge := strings.Repeat("a", 128*1024)
	r := httptest.NewRequest(http.MethodPost, "/_csp-report", strings.NewReader(huge))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("oversized body: want 204, got %d", w.Code)
	}
}

func TestUnknownRouteReturns404Envelope(t *testing.T) {
	h := router.New(router.Deps{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/no/such/path", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "route.not_found" {
		t.Errorf("want error=route.not_found, got %q", env.Error)
	}
}

func TestMethodNotAllowedReturns405Envelope(t *testing.T) {
	h := router.New(router.Deps{})

	// GET /healthz exists; DELETE does not.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/healthz", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "route.method_not_allowed" {
		t.Errorf("want error=route.method_not_allowed, got %q", env.Error)
	}
}

func TestNilMountHooks404(t *testing.T) {
	h := router.New(router.Deps{}) // all hooks nil

	paths := []string{"/api/anything", "/git/repo.git", "/mcp", "/ws/sessions/abc"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, p, nil)
			h.ServeHTTP(w, r)

			if w.Code != http.StatusNotFound {
				t.Errorf("path %q: want 404, got %d", p, w.Code)
			}
			env := decodeEnvelope(t, w.Body.String())
			if env.Error != "route.not_found" {
				t.Errorf("path %q: want route.not_found, got %q", p, env.Error)
			}
		})
	}
}

func TestMountAPIHookReached(t *testing.T) {
	var called bool
	h := router.New(router.Deps{
		Mounts: router.Mounts{
			API: func(r chi.Router) {
				r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusOK)
				})
			},
		},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	h.ServeHTTP(w, r)

	if !called {
		t.Error("MountAPI handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestMountGitHookReached(t *testing.T) {
	var called bool
	h := router.New(router.Deps{
		Mounts: router.Mounts{
			Git: func(r chi.Router) {
				r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
					called = true
					w.WriteHeader(http.StatusOK)
				})
			},
		},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/git/org/repo.git/info/refs", nil)
	h.ServeHTTP(w, r)

	if !called {
		t.Error("MountGit handler was not called")
	}
}

func TestMountMCPHookReached(t *testing.T) {
	var called bool
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	h := router.New(router.Deps{Mounts: router.Mounts{MCP: mcpHandler}})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	h.ServeHTTP(w, r)

	if !called {
		t.Error("MountMCP handler was not called")
	}
}

func TestMountWSHookReached(t *testing.T) {
	var called bool
	h := router.New(router.Deps{
		Mounts: router.Mounts{
			WS: func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusSwitchingProtocols)
			},
		},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws/sessions/test-session-id", nil)
	h.ServeHTTP(w, r)

	if !called {
		t.Error("MountWS handler was not called")
	}
}

func TestPanicInHandler(t *testing.T) {
	h := router.New(router.Deps{
		Mounts: router.Mounts{
			API: func(r chi.Router) {
				r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
					panic("deliberate panic for test")
				})
			},
		},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/panic", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Error != "internal" {
		t.Errorf("want error=internal, got %q", env.Error)
	}
}

func TestTrustProxyHeadersOff(t *testing.T) {
	// Without TrustProxyHeaders, the router should still function normally.
	h := router.New(router.Deps{Security: router.Security{TrustProxyHeaders: false}})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestHealthzContentType(t *testing.T) {
	h := router.New(router.Deps{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(w, r)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("want application/json content-type, got %q", ct)
	}
}
