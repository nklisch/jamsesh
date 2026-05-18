---
id: gate-security-security-headers-middleware
kind: story
stage: implementing
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
