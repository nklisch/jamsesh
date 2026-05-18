---
id: gate-security-security-headers-middleware
kind: story
stage: review
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# No Content-Security-Policy / X-Frame-Options / HSTS / X-Content-Type-Options headers

## Severity
Critical

## Domain
API Security

## Location
`internal/portal/router/router.go:73-86` (entire middleware stack)

## Evidence
```go
r.Use(chimw.RequestID)
if d.TrustProxyHeaders {
    r.Use(chimw.RealIP)
}
r.Use(logging.Access(d.MetricsRegistry))
r.Use(httperr.Recoverer)
```

No middleware sets `Content-Security-Policy`, `X-Frame-Options: DENY`,
`Strict-Transport-Security`, or `X-Content-Type-Options: nosniff`. The
SPA served at `/` has no CSP, so the
`gate-security-xss-html-render-ws-events` XSS is unmitigated; the portal
is also vulnerable to clickjacking on the org/session admin surfaces.

## Remediation direction
Add a security-headers middleware mounted globally that sets at minimum:
- `Content-Security-Policy: default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`
- `X-Content-Type-Options: nosniff`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains` (only when `tls.mode == native` or trusted proxy is HTTPS)
- `Referrer-Policy: no-referrer`

## Implementation notes

### Files changed
- **New**: `internal/portal/router/security_headers.go` — `SecurityHeaders(SecurityHeadersOptions)` middleware factory.
- **Modified**: `internal/portal/router/router.go` — added `TLSMode string` to `Deps`; mounted `SecurityHeaders` as the **first** `r.Use(...)` call at line ~97, before `chimw.RequestID`. The rate-limiting story (`gate-security-rate-limit-auth-endpoints`) should add its middleware to the `/api` route group, not to global `r.Use`, to avoid conflicts.
- **Modified**: `cmd/portal/main.go` — passes `TLSMode: cfg.TLS.Mode` into `router.Deps`.

### CSP policy
```
default-src 'self';
script-src 'self';
style-src 'self' 'unsafe-inline';
img-src 'self' data:;
font-src 'self' data:;
connect-src 'self';
object-src 'none';
base-uri 'none';
frame-ancestors 'none';
form-action 'self'
```

- `script-src 'self'` (strict): the Vite/Svelte build emits only `<script type="module" src="...">` references; no inline `<script>` blocks in `internal/portal/assets/dist/index.html`. If a future build introduces inline scripts, `'unsafe-inline'` must be added to `script-src` and tracked as a follow-on cleanup.
- `style-src 'unsafe-inline'`: Svelte scoped-CSS uses runtime `<style>` injections; nonce-based enforcement is deferred.
- `connect-src 'self'`: covers same-origin XHR, fetch, and WebSocket (`ws://`/`wss://` to the same origin count as `'self'` per spec).
- `frame-ancestors 'none'`: clickjacking defence for modern browsers; `X-Frame-Options: DENY` is the legacy complement.

### HSTS gating logic
```
enableHSTS = (TLSMode == "native") || (TLSMode == "behind_proxy" && TrustProxyHeaders)
```
- `"native"`: portal terminates TLS — HTTPS guaranteed, HSTS safe.
- `"behind_proxy"` + `TrustProxyHeaders=true`: operator signals trusted HTTPS proxy — HSTS safe.
- Any other combination (empty TLSMode, or `behind_proxy` without trust): HSTS is **not** emitted to avoid bricking HTTP-only dev/test setups.

### Middleware mount point
`r.Use(SecurityHeaders(...))` is the **first** `r.Use` call in `New`, before `chimw.RequestID`. This ensures security headers appear on all responses including error envelopes from `httperr.Recoverer` and logging middleware.

### Tests
All existing `go test ./internal/portal/...` tests pass. The golden `/metrics` e2e test (`tests/e2e/golden/metrics_endpoint_test.go`) asserts on Prometheus content-type and family presence — not on header count — so the new security headers on `/metrics` are compatible without assertion changes.
