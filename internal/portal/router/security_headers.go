package router

import "net/http"

// SecurityHeadersOptions controls which conditional headers are emitted.
type SecurityHeadersOptions struct {
	// EnableHSTS sets Strict-Transport-Security on every response.
	// Should only be true when the origin is HTTPS (native TLS or trusted
	// HTTPS proxy). Setting this over plain HTTP bricks HTTP clients.
	EnableHSTS bool

	// CSP overrides the default Content-Security-Policy directive string.
	// When empty, defaultCSP() is used.
	CSP string
}

// defaultCSP returns the baseline Content-Security-Policy for the portal SPA.
//
// Design notes:
//   - script-src 'self': the Vite/Svelte build emits only <script src=...>
//     references (no inline <script> blocks); verified against
//     internal/portal/assets/dist/index.html. If the build ever adds inline
//     scripts, 'unsafe-inline' must be added here and documented.
//   - style-src 'unsafe-inline': Svelte scoped-CSS blocks emit runtime <style>
//     injections; nonce-based enforcement is out of scope for this story.
//   - connect-src 'self': covers same-origin XHR, fetch, and WebSocket
//     (ws:// / wss:// to the same origin are treated as 'self' per spec).
//   - frame-ancestors 'none': redundant with X-Frame-Options: DENY but covers
//     modern browsers that honour CSP over X-Frame-Options.
func defaultCSP() string {
	return "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'none'; " +
		"form-action 'self'"
}

// cspReportOnly returns the Content-Security-Policy-Report-Only directive
// string. It mirrors defaultCSP() but adds a report-uri directive so that
// inline-script regressions surface in server logs without breaking the page.
//
// The /_csp-report endpoint is a placeholder; see backlog item
// bug-csp-report-endpoint-not-wired for wiring an actual report receiver.
// Until the receiver is wired, browsers will POST reports to /_csp-report
// and receive a 404, which is harmless — the reports are still emitted to
// the browser's console and any devtools network panel.
func cspReportOnly() string {
	return defaultCSP() + "; report-uri /_csp-report"
}

// SecurityHeaders returns a middleware that sets standard security headers on
// every response. It should be mounted globally at the start of the middleware
// stack, before RequestID, so all routes — including /healthz, /metrics, and
// the SPA catch-all — receive the headers.
func SecurityHeaders(opts SecurityHeadersOptions) func(http.Handler) http.Handler {
	csp := opts.CSP
	if csp == "" {
		csp = defaultCSP()
	}
	cspRO := cspReportOnly()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			// Report-Only header: mirrors the enforced CSP with report-uri added.
			// Inline-script violations (e.g. a future Svelte build regression that
			// emits an inline <script>) are reported to /_csp-report without
			// breaking the page, catching XSS-mitigation regressions early.
			h.Set("Content-Security-Policy-Report-Only", cspRO)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if opts.EnableHSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
