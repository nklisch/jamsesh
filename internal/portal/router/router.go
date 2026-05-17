// Package router assembles the portal's chi router from pluggable mount hooks.
// Each sibling feature populates one or more hooks in Deps; missing hooks
// transparently 404 through the standard JSON envelope.
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/logging"
)

// Deps is the dependency surface for the portal router. Concrete handler
// interfaces (TokenStore, MCP handler, etc.) land here as sibling features
// ship. All mount hooks are nilable so this package does not hard-depend on
// features that haven't landed yet.
type Deps struct {
	// TrustProxyHeaders enables chi's RealIP middleware, which honours
	// X-Forwarded-For and X-Real-IP. Set only when running behind a
	// trusted reverse proxy; direct-listen mode must not trust these headers.
	TrustProxyHeaders bool

	// Mount hooks — left as nilable so http-skeleton can ship before its
	// dependents. Missing hooks 404 through the envelope.
	MountAPI func(chi.Router) // owned by tokens + auth-flows + accounts
	MountGit func(chi.Router) // owned by epic-portal-git
	MountMCP http.Handler     // owned by epic-portal-api
	MountWS  http.HandlerFunc // owned by epic-portal-api
	// MountUI serves the embedded Svelte SPA at / as a catch-all after all
	// other routes. Owned by epic-portal-ui-foundation (assets package).
	// Must be registered last so API/git/mcp/ws routes take precedence.
	MountUI http.Handler // owned by epic-portal-ui-foundation

	// MountTest is a nilable hook for test-only routes under /test/*.
	// Populated only by the e2etest-tagged binary (see
	// cmd/portal/test_clock_advance.go); production builds leave it nil
	// and the /test subtree is never registered. The build-tag gate in
	// cmd/portal is the trust boundary for this hook.
	MountTest func(chi.Router)
}

// New returns the root http.Handler for the portal. Middleware order is
// intentional:
//
//  1. RequestID  — every downstream log line carries the request ID
//  2. RealIP     — gated on TrustProxyHeaders (see Deps)
//  3. Access     — logs after the full response is written
//  4. Recoverer  — converts panics to the JSON envelope
//
// Route groups mount after the global middleware and may declare their own
// per-group middleware (auth, etc.) inside their mount hooks.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Override chi's default text/plain 404 and 405 responses.
	r.NotFound(httperr.NotFoundHandler().ServeHTTP)
	r.MethodNotAllowed(httperr.MethodNotAllowedHandler().ServeHTTP)

	// Global middleware stack — order matters.
	r.Use(chimw.RequestID)
	if d.TrustProxyHeaders {
		r.Use(chimw.RealIP)
	}
	r.Use(logging.Access)
	r.Use(httperr.Recoverer)

	// Public liveness probe — no auth.
	r.Get("/healthz", healthz)

	// /api — REST API, Bearer auth. Hook is responsible for attaching
	// auth middleware and mounting oapi-codegen handlers.
	r.Route("/api", func(r chi.Router) {
		if d.MountAPI != nil {
			d.MountAPI(r)
		}
	})

	// /git — smart-HTTP, HTTP Basic auth.
	if d.MountGit != nil {
		r.Route("/git", d.MountGit)
	}

	// /mcp — MCP SDK handler, bearer auth per-request inside the SDK.
	if d.MountMCP != nil {
		r.Mount("/mcp", d.MountMCP)
	}

	// /test — test-only mutators (e.g. POST /test/clock-advance) registered
	// exclusively by the e2etest-tagged portal binary. Mounted BEFORE the
	// SPA catch-all so the /test/* paths take precedence; production builds
	// pass nil and the subtree is never registered.
	if d.MountTest != nil {
		r.Route("/test", d.MountTest)
	}

	// /ws — WebSocket upgrade; auth happens at upgrade time inside the handler.
	if d.MountWS != nil {
		r.Get("/ws/sessions/{sessionID}", d.MountWS)
	}

	// / — SPA catch-all. Must be last so all named routes above take precedence.
	// Serves the embedded Svelte bundle; falls back to index.html for History-API
	// deep links. When MountUI is nil, the chi NotFound handler applies.
	if d.MountUI != nil {
		r.Mount("/", d.MountUI)
	}

	return r
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
