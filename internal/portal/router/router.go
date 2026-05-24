// Package router assembles the portal's chi router from pluggable mount hooks.
// Each sibling feature populates one or more hooks in Deps; missing hooks
// transparently 404 through the standard JSON envelope.
package router

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"jamsesh/internal/buildinfo"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/logging"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/probes"
)

// Security groups TLS posture and proxy-trust settings.
type Security struct {
	// TLSMode mirrors config.TLSConfig.Mode ("native" | "behind_proxy").
	// Used by the security-headers middleware to decide whether to emit HSTS:
	// "native" means the portal itself terminates TLS, so HSTS is safe;
	// "behind_proxy" defers to TrustProxyHeaders to determine whether the
	// proxy terminates HTTPS. Empty string is treated as non-HTTPS (no HSTS).
	TLSMode string

	// TrustProxyHeaders enables chi's RealIP middleware, which honours
	// X-Forwarded-For and X-Real-IP. Set only when running behind a
	// trusted reverse proxy; direct-listen mode must not trust these headers.
	TrustProxyHeaders bool
}

// Mounts groups the nilable mount hooks for each subsystem. Missing hooks
// 404 through the standard JSON envelope.
type Mounts struct {
	// API is owned by tokens + auth-flows + accounts.
	API func(chi.Router)
	// MCP is the MCP SDK handler, owned by epic-portal-api.
	MCP http.Handler
	// Git is owned by epic-portal-git.
	Git func(chi.Router)
	// WS is the WebSocket upgrade handler, owned by epic-portal-api.
	WS http.HandlerFunc
	// UI serves the embedded Svelte SPA at / as a catch-all after all
	// other routes. Owned by epic-portal-ui-foundation (assets package).
	// Must be registered last so API/git/mcp/ws routes take precedence.
	UI http.Handler
	// Test is a nilable hook for test-only routes under /test/*.
	// Populated only by the e2etest-tagged binary (see
	// cmd/portal/test_clock_advance.go); production builds leave it nil
	// and the /test subtree is never registered. The build-tag gate in
	// cmd/portal is the trust boundary for this hook.
	Test func(chi.Router)
}

// Probes groups readiness probe configuration.
type Probes struct {
	// Ready is the list of readiness probes mounted at /readyz.
	// When nil or empty, the /readyz route is not registered and the path
	// falls through to the 404 handler. Populated in main.go with DB ping
	// and storage stat checks.
	Ready []probes.Check
}

// Metrics groups the metrics endpoint configuration.
type Metrics struct {
	// Handler serves GET /metrics in Prometheus text exposition format.
	// When nil or when Token is empty, the /metrics route is not
	// registered. Access is gated behind Token bearer authentication.
	Handler http.Handler

	// Token is the static bearer token required to access /metrics.
	// When empty (the default), /metrics is not mounted at all — operators
	// must explicitly opt in via JAMSESH_METRICS_TOKEN. When set, the handler
	// is wrapped with a constant-time bearer check; missing or mismatched
	// tokens receive 401.
	Token string

	// Registry is threaded into the Access logging middleware so that
	// per-request counters and histograms are recorded. When nil, the Access
	// middleware logs normally but skips metric recording.
	Registry *metrics.Registry
}

// Deps is the dependency surface for the portal router. Concrete handler
// interfaces (TokenStore, MCP handler, etc.) land here as sibling features
// ship. All mount hooks are nilable so this package does not hard-depend on
// features that haven't landed yet.
type Deps struct {
	Security Security
	Mounts   Mounts
	Probes   Probes
	Metrics  Metrics

	// APIBodyLimitBytes is the maximum number of bytes the server will read
	// from any request body on /api/* routes. Requests whose bodies exceed
	// this limit are rejected with 413 Request Entity Too Large before the
	// handler can decode them.
	// Default (when zero): 1 MiB (1 << 20).
	// Set via JAMSESH_API_BODY_LIMIT_BYTES or config.APIBodyLimitBytes.
	// Git smart-HTTP routes (/git/*) are NOT affected — they manage their
	// own per-route limits.
	APIBodyLimitBytes int64
}

// New returns the root http.Handler for the portal. Middleware order is
// intentional:
//
//  1. SecurityHeaders — CSP, X-Frame-Options, HSTS (when HTTPS), etc.
//  2. RequestID       — every downstream log line carries the request ID
//  3. RealIP          — gated on Security.TrustProxyHeaders (see Deps)
//  4. Access          — logs after the full response is written
//  5. Recoverer       — converts panics to the JSON envelope
//
// Route groups mount after the global middleware and may declare their own
// per-group middleware (auth, etc.) inside their mount hooks. The /api group
// additionally applies BodyLimit (default 1 MiB) as its first middleware so
// oversized bodies are rejected before any handler decoding occurs.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Override chi's default text/plain 404 and 405 responses.
	r.NotFound(httperr.NotFoundHandler().ServeHTTP)
	r.MethodNotAllowed(httperr.MethodNotAllowedHandler().ServeHTTP)

	// enableHSTS is true when the origin is reliably served over HTTPS:
	//   - "native": the portal terminates TLS itself.
	//   - "behind_proxy" + TrustProxyHeaders: operator asserts the upstream
	//     proxy terminates HTTPS. (TrustProxyHeaders is the operator signal.)
	enableHSTS := d.Security.TLSMode == "native" || (d.Security.TLSMode == "behind_proxy" && d.Security.TrustProxyHeaders)

	// Global middleware stack — order matters.
	// SecurityHeaders is first so every response — including error responses
	// generated by later middleware — carries the security headers.
	// Mounted at line 97 in router.go; rate-limiting for /auth/* goes on the
	// /api route group (see gate-security-rate-limit-auth-endpoints story).
	r.Use(SecurityHeaders(SecurityHeadersOptions{EnableHSTS: enableHSTS}))
	r.Use(chimw.RequestID)
	if d.Security.TrustProxyHeaders {
		r.Use(chimw.RealIP)
	}
	r.Use(logging.Access(d.Metrics.Registry))
	r.Use(httperr.Recoverer)

	// Public liveness probe — no auth.
	r.Get("/healthz", healthz)

	// Public readiness probe — only registered when checks are configured.
	if len(d.Probes.Ready) > 0 {
		r.Get("/readyz", probes.Handler(d.Probes.Ready).ServeHTTP)
	}

	// Metrics endpoint — only mounted when a token is configured (opt-in).
	// Access is gated behind a static bearer-token check using constant-time
	// comparison to prevent timing attacks. Operators set JAMSESH_METRICS_TOKEN;
	// unset means /metrics is not registered and the path falls through to 404.
	if d.Metrics.Handler != nil && len(d.Metrics.Token) > 0 {
		r.Mount("/metrics", metricsTokenMiddleware(d.Metrics.Token, d.Metrics.Handler))
	}

	// /api — REST API, Bearer auth. Hook is responsible for attaching
	// auth middleware and mounting oapi-codegen handlers.
	// BodyLimit is applied here so every /api/* handler is capped at
	// APIBodyLimitBytes (default 1 MiB). Git smart-HTTP (/git/*) has its own
	// per-route limits and is NOT affected.
	apiBodyLimit := d.APIBodyLimitBytes
	if apiBodyLimit <= 0 {
		apiBodyLimit = 1 << 20 // 1 MiB default
	}
	r.Route("/api", func(r chi.Router) {
		r.Use(BodyLimit(apiBodyLimit))
		if d.Mounts.API != nil {
			d.Mounts.API(r)
		}
	})

	// /git — smart-HTTP, HTTP Basic auth.
	if d.Mounts.Git != nil {
		r.Route("/git", d.Mounts.Git)
	}

	// /mcp — MCP SDK handler, bearer auth per-request inside the SDK.
	if d.Mounts.MCP != nil {
		r.Mount("/mcp", d.Mounts.MCP)
	}

	// /test — test-only mutators (e.g. POST /test/clock-advance) registered
	// exclusively by the e2etest-tagged portal binary. Mounted BEFORE the
	// SPA catch-all so the /test/* paths take precedence; production builds
	// pass nil and the subtree is never registered.
	if d.Mounts.Test != nil {
		r.Route("/test", d.Mounts.Test)
	}

	// /ws — WebSocket upgrade; auth happens at upgrade time inside the handler.
	if d.Mounts.WS != nil {
		r.Get("/ws/sessions/{sessionID}", d.Mounts.WS)
	}

	// / — SPA catch-all. Must be last so all named routes above take precedence.
	// Serves the embedded Svelte bundle; falls back to index.html for History-API
	// deep links. When Mounts.UI is nil, the chi NotFound handler applies.
	if d.Mounts.UI != nil {
		r.Mount("/", d.Mounts.UI)
	}

	return r
}

// metricsTokenMiddleware wraps next with a static bearer-token check.
// It extracts the token from the "Authorization: Bearer <token>" header
// and compares it against wantToken using subtle.ConstantTimeCompare to
// prevent timing side-channels. Missing or mismatched tokens return 401.
func metricsTokenMiddleware(wantToken string, next http.Handler) http.Handler {
	wantBytes := []byte(wantToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), wantBytes) != 1 {
			httperr.Write(w, r, &httperr.Error{
				Code:       "auth.invalid_token",
				Message:    "valid bearer token required",
				HTTPStatus: http.StatusUnauthorized,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": buildinfo.String(),
	})
}
